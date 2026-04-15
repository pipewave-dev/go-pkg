package activeConnRepo

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	activeConnExp "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn/exprbuilder"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

const fnCleanUpExpiredConnections = "activeConnRepo.CleanUpExpiredConnections"

func (r *activeConnRepo) CleanUpExpiredConnections(ctx context.Context) (aErr aerror.AError) {
	var op observer.Operation
	ctx, op = r.obs.StartOperation(ctx, fnCleanUpExpiredConnections)
	defer op.Finish(aErr)

	filterEx := expression.Name(activeConnExp.FieldTTL).LessThan(expression.Value(time.Now().UnixMilli()))
	expr, err := expression.NewBuilder().WithFilter(filterEx).Build()
	if err != nil {
		panic(fmt.Sprintf("activeConnRepo.CleanUpExpiredConnections build expression error: %v", err))
	}

	//nolint:exhaustruct
	scanInput := &dynamodb.ScanInput{
		TableName:                 lo.ToPtr(r.c.Env().DynamoDB.Tables.ActiveConnection),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ProjectionExpression:      lo.ToPtr(activeConnExp.FieldUserID + ", " + activeConnExp.FieldInstanceID),
	}

	type itemKey struct {
		UserID     string
		InstanceID string
	}

	var keys []itemKey
	paginator := dynamodb.NewScanPaginator(r.ddb.Client(), scanInput)
	for paginator.HasMorePages() {
		var output *dynamodb.ScanOutput
		output, err = paginator.NextPage(ctx)
		if err != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
			return aErr
		}

		for _, item := range output.Items {
			var key itemKey
			if err = attributevalue.UnmarshalMap(item, &key); err != nil {
				aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
				return aErr
			}
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return nil
	}

	tableName := r.c.Env().DynamoDB.Tables.ActiveConnection

	writeReqs := make([]types.WriteRequest, 0, len(keys))
	for _, key := range keys {
		keyAV, err := attributevalue.MarshalMap(key)
		if err != nil {
			panic(fmt.Sprintf("activeConnRepo.CleanUpExpiredConnections marshal key error: %v", err))
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
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, fmt.Errorf("failed to delete %d expired active connections", len(unprocessedItems)))
		return aErr
	}

	return nil
}
