package pendingMessageRepo

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

const fnCreate = "pendingMessageRepo.Create"

type ddbPendingMessage struct {
	SessionKey string // PK: userID:instanceID
	SendAt     int64  // SK: Unix nano
	Message    []byte
	TTL        int64 // Unix seconds for DynamoDB TTL
}

func (r *pendingMessageRepo) Create(ctx context.Context, userID, instanceID string, sendAt time.Time, message []byte) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCreate)
	defer op.Finish(aErr)

	ttl := time.Now().Add(r.c.Env().MessageHub.TTL).Unix()

	item := ddbPendingMessage{
		SessionKey: sessionKey(userID, instanceID),
		SendAt:     sendAt.UnixNano(),
		Message:    message,
		TTL:        ttl,
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		panic(fmt.Sprintf("pendingMessageRepo.Create marshal error: %v", err))
	}

	//nolint:exhaustruct
	input := &dynamodb.PutItemInput{
		TableName: lo.ToPtr(r.c.Env().DynamoDB.Tables.PendingMessage),
		Item:      av,
	}

	_, err2 := r.ddbC.PutItem(ctx, input)
	if err2 != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
		return aErr
	}

	return nil
}
