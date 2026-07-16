package pendingMessageRepo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

const fnDeleteUpTo = "pendingMessageRepo.DeleteUpTo"

func (r *pendingMessageRepo) DeleteUpTo(ctx context.Context, userID, instanceID string, maxSendAt int64) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnDeleteUpTo)
	defer op.Finish(aErr)

	// Key condition on both SessionKey (hash) and SendAt (sort key, range), so only the
	// messages that existed at read time are targeted — a message Created after the read
	// (SendAt > maxSendAt) is left untouched.
	keyEx := expression.Key("SessionKey").Equal(expression.Value(sessionKey(userID, instanceID))).
		And(expression.Key("SendAt").LessThanEqual(expression.Value(maxSendAt)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		panic(fmt.Sprintf("pendingMessageRepo.DeleteUpTo build expression error: %v", err))
	}

	//nolint:exhaustruct
	queryInput := &dynamodb.QueryInput{
		TableName:                 lo.ToPtr(r.c.Env().DynamoDB.Tables.PendingMessage),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ProjectionExpression:      lo.ToPtr("SessionKey, SendAt"),
	}

	type itemKey struct {
		SessionKey string
		SendAt     int64
	}

	var keys []itemKey
	paginator := dynamodb.NewQueryPaginator(r.ddb.Client(), queryInput)
	for paginator.HasMorePages() {
		output, err2 := paginator.NextPage(ctx)
		if err2 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
			return aErr
		}
		for _, item := range output.Items {
			var k itemKey
			if err3 := attributevalue.UnmarshalMap(item, &k); err3 != nil {
				aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err3)
				return aErr
			}
			keys = append(keys, k)
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// BatchWriteItem in chunks of 25 (DynamoDB limit)
	tableName := r.c.Env().DynamoDB.Tables.PendingMessage

	writeReqs := make([]types.WriteRequest, 0, len(keys))
	for _, k := range keys {
		keyAV, err4 := attributevalue.MarshalMap(k)
		if err4 != nil {
			panic(fmt.Sprintf("pendingMessageRepo.DeleteUpTo marshal key error: %v", err4))
		}

		writeReqs = append(writeReqs, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{Key: keyAV},
		})
	}

	unprocessedItems, err := r.ddb.RecursiveBatchWriteItem(ctx, tableName, writeReqs, 2)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return aErr
	}
	if len(unprocessedItems) > 0 {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, fmt.Errorf("failed to delete %d pending messages", len(unprocessedItems)))
		return aErr
	}

	return nil
}
