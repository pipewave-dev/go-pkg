package repository

import (
	"context"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

// PendingMessageRepo stores pre-wrapped WebSocket response bytes for temporarily disconnected sessions.
//
// DynamoDB table structure:
//   - Hash key:  userID + ":" + instanceID
//   - Sort key:  sendAt (Unix nano int64) — GetAll returns ascending order
//   - TTL attr:  same duration as the session temp-disconnect TTL from config
type PendingMessageRepo interface {
	Create(ctx context.Context, userID, instanceID string, sendAt time.Time, message []byte) aerror.AError
	// GetAll returns pending messages ordered by SendAt ascending, along with the SendAt
	// (Unix nano) of the last message returned (0 if none). Pair with DeleteUpTo, not
	// DeleteAll, to avoid deleting messages Created concurrently after this read.
	GetAll(ctx context.Context, userID, instanceID string) (msgs [][]byte, maxSendAt int64, aErr aerror.AError)
	// DeleteUpTo deletes only pending messages with SendAt <= maxSendAt, so it can safely
	// follow GetAll without racing a concurrent Create for the same session.
	DeleteUpTo(ctx context.Context, userID, instanceID string, maxSendAt int64) aerror.AError
	DeleteAll(ctx context.Context, userID, instanceID string) aerror.AError
	CleanUpExpiredPendingMessages(ctx context.Context) aerror.AError
}
