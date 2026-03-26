package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

type ActiveConnectionQuerier struct {
	ConfigStore configprovider.ConfigStore
}

/*
CountActiveParams will count connections of UserID that have a LastHeartbeat within the recent CutOffTime. For example:
- If CutOffTime = 10 minutes, it will count connections with a LastHeartbeat within the last 10 minutes, and ignore connections that are no longer active with a LastHeartbeat > 10 minutes.
*/
type CountActiveParams struct {
	UserID         string
	CutOffDuration time.Duration
}

func (querier *ActiveConnectionQuerier) CountActive(ctx context.Context, ddbClient *dynamodb.Client, params CountActiveParams) (count int, aErr aerror.AError) {
	// Build query expression - query by partition key only
	keyEx := expression.Key(FieldUserID).Equal(expression.Value(params.UserID))

	cutoffTime := time.Now().Add(-params.CutOffDuration)

	filterEx := expression.Name(FieldLastHeartbeat).GreaterThan(expression.Value(cutoffTime))

	builder := expression.NewBuilder().
		WithKeyCondition(keyEx).
		WithFilter(filterEx)

	expr, errB := builder.Build()
	if errB != nil {
		msg := fmt.Sprintf("ActiveConnectionQuerier.CountActive failed: %v", errB)
		panic(msg)
	}

	// Execute query
	//nolint:exhaustruct
	queryParams := &dynamodb.QueryInput{
		TableName:                 lo.ToPtr(querier.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		// ExclusiveStartKey:         nil,
	}

	paginator := dynamodb.NewQueryPaginator(ddbClient, queryParams)

	count = 0
	for paginator.HasMorePages() {
		output, err1 := paginator.NextPage(ctx)
		if err1 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err1)
			return 0, aErr
		}

		count += len(output.Items)
	}

	return count, nil
}

//

/*
CountTotalActiveParams will count connections that have a LastHeartbeat within the recent CutOffTime. For example:
- If CutOffTime = 10 minutes, it will count connections with a LastHeartbeat within the last 10 minutes, and ignore connections that are no longer active with a LastHeartbeat > 10 minutes.
*/
type CountTotalActiveParams struct {
	CutOffDuration time.Duration
}

func (querier *ActiveConnectionQuerier) CountTotalActive(ctx context.Context, ddbClient *dynamodb.Client, params CountTotalActiveParams) (total int64, aErr aerror.AError) {
	tablename := querier.ConfigStore.Env().DynamoDB.Tables.ActiveConnection
	cutoffTime := time.Now().Add(-params.CutOffDuration)

	filterEx := expression.Name(FieldLastHeartbeat).GreaterThan(expression.Value(cutoffTime))

	expr, errB := expression.NewBuilder().WithFilter(filterEx).Build()
	if errB != nil {
		msg := fmt.Sprintf("ActiveConnectionQuerier.CountTotalActive failed: %v", errB)
		panic(msg)
	}

	//nolint:exhaustruct
	scanParams := &dynamodb.ScanInput{
		TableName:                 lo.ToPtr(tablename),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Select:                    types.SelectCount,
	}

	paginator := dynamodb.NewScanPaginator(ddbClient, scanParams)

	total = int64(0)
	for paginator.HasMorePages() {
		output, err1 := paginator.NextPage(ctx)
		if err1 != nil {
			aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err1)
			return 0, aErr
		}

		total += int64(output.Count)
	}

	return total, nil
}

// QueryByUserID returns all active connections for a user.
func (querier *ActiveConnectionQuerier) QueryByUserID(ctx context.Context, ddbClient *dynamodb.Client, userID string) ([]entities.ActiveConnection, aerror.AError) {
	keyEx := expression.Key(FieldUserID).Equal(expression.Value(userID))

	cutoffTime := time.Now().Add(-querier.ConfigStore.Env().HeartbeatCutoff)
	filterEx := expression.Name(FieldLastHeartbeat).GreaterThan(expression.Value(cutoffTime))

	builder := expression.NewBuilder().
		WithKeyCondition(keyEx).
		WithFilter(filterEx)

	expr, errB := builder.Build()
	if errB != nil {
		msg := fmt.Sprintf("ActiveConnectionQuerier.QueryByUserID failed: %v", errB)
		panic(msg)
	}

	//nolint:exhaustruct
	queryParams := &dynamodb.QueryInput{
		TableName:                 lo.ToPtr(querier.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		ConsistentRead:            lo.ToPtr(true),
	}

	paginator := dynamodb.NewQueryPaginator(ddbClient, queryParams)

	var results []entities.ActiveConnection
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		}

		for _, item := range output.Items {
			entity, err := fromDynamoMap(item)
			if err != nil {
				return nil, aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
			}
			results = append(results, *entity)
		}
	}

	return results, nil
}

// GetByUserAndSession returns a single active connection by userID and sessionID.
func (querier *ActiveConnectionQuerier) GetByUserAndSession(ctx context.Context, ddbClient *dynamodb.Client, userID string, sessionID string) (*entities.ActiveConnection, aerror.AError) {
	type keySchema struct {
		UserID    string
		SessionID string
	}

	keyAV, err := attributevalue.MarshalMap(keySchema{UserID: userID, SessionID: sessionID})
	if err != nil {
		msg := fmt.Sprintf("ActiveConnectionQuerier.GetByUserAndSession marshal key failed: %v", err)
		panic(msg)
	}

	//nolint:exhaustruct
	input := &dynamodb.GetItemInput{
		TableName:      lo.ToPtr(querier.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:            keyAV,
		ConsistentRead: lo.ToPtr(true),
	}

	output, err2 := ddbClient.GetItem(ctx, input)
	if err2 != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
	}

	if len(output.Item) == 0 {
		return nil, aerror.New(ctx, aerror.RecordNotFound, nil)
	}

	entity, err3 := fromDynamoMap(output.Item)
	if err3 != nil {
		return nil, aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err3)
	}

	// If the connection's last heartbeat is too old, consider it as not found (stale connection)
	cutoffTime := time.Now().Add(-querier.ConfigStore.Env().HeartbeatCutoff)
	if entity.LastHeartbeat.Before(cutoffTime) {
		return nil, aerror.New(ctx, aerror.RecordNotFound, nil)
	}

	return entity, nil
}
