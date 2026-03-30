package mediatorsvc

import (
	"context"
	"log/slog"
)

// Shutdown performs graceful shutdown of the mediator service.
//
// Order of operations:
//  1. MarkShuttingDown — signals onClose to skip DB operations (we handle them here).
//  2. Cancel pending ACKs — unblocks goroutines waiting in WaitForAck.
//  3. For each authenticated connection:
//     a. UpdateStatusTransferring — atomically sets Status=Transferring, HolderID="".
//     b. Register in MessageHub — buffers messages for the session until TTL.
//  4. Close all connections — triggers onClose which skips DB ops (already handled above).
//
// Close is done last so that if DB updates are slow, the WebSocket remains open
// to receive and buffer messages until the record is committed.
func (m *mediatorSvc) Shutdown() {
	ctx := context.Background()

	// 1. Signal delivery layer: use shutdown path for all subsequent closes.
	m.shutdownSignal.MarkShuttingDown()

	// 2. Cancel all pending ACKs so goroutines blocked in WaitForAck are unblocked immediately.
	m.ackManager.Shutdown()

	// 3. Update all authenticated connections to WsStatusTransferring and register in MessageHub.
	allAuthConns := m.connections.GetAllAuthenticatedConn()
	for _, conn := range allAuthConns {
		auth := conn.Auth()
		if auth.IsAnonymous() {
			continue
		}

		// Atomically set Status=Transferring and clear HolderID so any container can claim on reconnect.
		if aErr := m.activeConnRepo.UpdateStatusTransferring(ctx, auth.UserID, auth.InstanceID); aErr != nil {
			slog.ErrorContext(ctx, "Shutdown: UpdateStatusTransferring failed — session may be lost",
				slog.String("userID", auth.UserID),
				slog.String("instanceID", auth.InstanceID),
				slog.Any("error", aErr))
			// Continue: best-effort. Other sessions should still be processed.
		}

		// Register in MessageHub so incoming messages are buffered until the client reconnects.
		// onExpired fires if TTL elapses without reconnect.
		// Guard: only remove the DB record if no other container has claimed the session
		// (HolderID still empty). If HolderID != "", the client reconnected elsewhere — skip removal.
		userID := auth.UserID
		instanceID := auth.InstanceID
		m.msgHubSvc.Register(userID, instanceID, func() {
			actConn, aErr := m.activeConnRepo.GetInstanceConnection(ctx, userID, instanceID)
			if aErr != nil || actConn == nil {
				return // Already removed or not found — nothing to do.
			}
			if actConn.HolderID != "" {
				// Another container claimed the session. Don't remove.
				return
			}
			if err := m.activeConnRepo.RemoveConnection(ctx, userID, instanceID); err != nil {
				slog.ErrorContext(ctx, "Shutdown.onExpired: failed to remove ActiveConnection",
					slog.String("userID", userID),
					slog.String("instanceID", instanceID),
					slog.Any("error", err))
			}
		})
	}

	// 4. Close all connections — onClose skips DB ops (MarkShuttingDown was called above).
	for _, conn := range m.connections.GetAllConnections() {
		conn.Close()
	}
}
