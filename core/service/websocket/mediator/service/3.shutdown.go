package mediatorsvc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/shared/actx"
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

	// 3. Close all anonymous connections immediately — no DB state to worry about.
	m.closeAllAnonymousConnections()

	// 4. Update all authenticated connections to WsStatusTransferring and register in MessageHub.
	m.shutdownAuthenticatedConnections(ctx)
	// 5. Close all authenticated connections — onClose skips DB ops (MarkShuttingDown was called above).
	m.closeAllAuthenticatedConnections()
}

// shutdownAuthenticatedConnections updates all authenticated connections to WsStatusTransferring
// and registers them in MessageHub so that messages are buffered until the client reconnects.
func (m *mediatorSvc) shutdownAuthenticatedConnections(ctx context.Context) {
	allAuthConns := m.connections.GetAllAuthenticatedConn()
	m.transferingConns = make([]connectionInfo, 0, len(allAuthConns))
	for _, conn := range allAuthConns {
		auth := conn.Auth()
		m.transferingConns = append(m.transferingConns, connectionInfo{
			userID:     auth.UserID,
			instanceID: auth.InstanceID,
		})
		m.transitionConnectionToTransferring(ctx, auth.UserID, auth.InstanceID)
	}
}

// transitionConnectionToTransferring atomically updates a connection to WsStatusTransferring
// and registers it in MessageHub for message buffering.
func (m *mediatorSvc) transitionConnectionToTransferring(ctx context.Context, userID, instanceID string) {
	// Atomically set Status=Transferring and clear HolderID so any container can claim on reconnect.
	if aErr := m.activeConnRepo.UpdateStatusTransferring(ctx, userID, instanceID); aErr != nil {
		slog.ErrorContext(ctx, "Shutdown: UpdateStatusTransferring failed — session may be lost",
			slog.String("userID", userID),
			slog.String("instanceID", instanceID),
			slog.Any("error", aErr))
		// Continue: best-effort. Other sessions should still be processed.
	}
}

// closeAllAuthenticatedConnections closes all authenticated WebSocket connections.

func (m *mediatorSvc) closeAllAuthenticatedConnections() {
	for _, conn := range m.connections.GetAllAuthenticatedConn() {
		conn.Close()
	}
}

func (m *mediatorSvc) checkTransferingConns() {
	ctx := actx.New()
	ctx.SetTraceID(
		fmt.Sprintf("shutdown%s", m.c.Env().Info.ContainerID))
	notTransferedConns := make([]connectionInfo, 0, len(m.transferingConns))
	for _, connInfo := range m.transferingConns {
		ac, err := m.activeConnRepo.GetInstanceConnection(ctx, connInfo.userID, connInfo.instanceID)
		if err != nil {
			slog.ErrorContext(ctx, "Shutdown: Failed to get instance connection",
				slog.String("userID", connInfo.userID),
				slog.String("instanceID", connInfo.instanceID),
				slog.Any("error", err))
			continue
		}
		if ac.Status == voWs.WsStatusTransferring {
			notTransferedConns = append(notTransferedConns, connectionInfo{
				userID:     ac.UserID,
				instanceID: ac.InstanceID,
			})
		}
	}
	if len(notTransferedConns) > 0 {
		connsStr := strings.Builder{}
		for _, conn := range notTransferedConns {
			connsStr.WriteString(conn.String())
			connsStr.WriteString("; ")
		}
		slog.WarnContext(ctx, "Some connection still stuck in Transfering status. This session is not reconnected from browser",
			slog.Any("connections", connsStr))
	}
}

// closeAllAnonymousConnections closes all anonymous WebSocket connections.
func (m *mediatorSvc) closeAllAnonymousConnections() {
	for _, conn := range m.connections.GetAllAnonymousConn() {
		conn.Close()
	}
}
