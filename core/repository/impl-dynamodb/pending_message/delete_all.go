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

const fnDeleteAll = "pendingMessageRepo.DeleteAll"

func (r *pendingMessageRepo) DeleteAll(ctx context.Context, userID, instanceID string) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnDeleteAll)
	defer op.Finish(aErr)

	// Query all items by SessionKey to collect keys for deletion
	keyEx := expression.Key("SessionKey").Equal(expression.Value(sessionKey(userID, instanceID)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		panic(fmt.Sprintf("pendingMessageRepo.DeleteAll build expression error: %v", err))
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
	paginator := dynamodb.NewQueryPaginator(r.ddbC, queryInput)
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
	const batchSize = 25

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[i:end]

		var writeReqs []types.WriteRequest
		for _, k := range chunk {
			keyAV, err4 := attributevalue.MarshalMap(k)
			if err4 != nil {
				panic(fmt.Sprintf("pendingMessageRepo.DeleteAll marshal key error: %v", err4))
			}
			writeReqs = append(writeReqs, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{Key: keyAV},
			})
		}

		//nolint:exhaustruct
		_, err5 := r.ddbC.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				tableName: writeReqs,
			},
		})
		if err5 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err5)
			return aErr
		}
	}

	return nil
}
