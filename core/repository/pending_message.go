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
	GetAll(ctx context.Context, userID, instanceID string) ([][]byte, aerror.AError)
	DeleteAll(ctx context.Context, userID, instanceID string) aerror.AError
	CleanUpExpiredPendingMessages(ctx context.Context) aerror.AError
}
