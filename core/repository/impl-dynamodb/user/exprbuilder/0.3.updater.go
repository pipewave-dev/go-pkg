package exprbuilder

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	repohelper "github.com/pipewave-dev/go-pkg/shared/utils/repo-helper"
	"github.com/samber/lo"
)

// UserUpdater updates User entity
type UserUpdater struct {
	ConfigStore configprovider.ConfigStore
}

type UpdateLastHeartbeatParams struct {
	ID              string
	LastHeartbeatAt time.Time
}

func (updater *UserUpdater) UpdateLastHeartbeat(ctx context.Context, ddbClient *dynamodb.Client, params UpdateLastHeartbeatParams) aerror.AError {
	key, err := attributevalue.MarshalMap(map[string]any{
		FieldID: params.ID,
	})
	if err != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
	}

	update := expression.
		Set(
			expression.Name(FieldLastHeartbeat),
			expression.Value(voUnixTime.UnixMilliTime(params.LastHeartbeatAt)))
	cond := expression.Name(FieldID).AttributeExists()

	expr, err := expression.NewBuilder().WithUpdate(update).WithCondition(cond).Build()
	if err != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
	}

	// Use UpdateItem instead of PutItem to avoid overwriting other fields (CreatedAt, etc.)
	//nolint:exhaustruct
	updateItemParams := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(updater.ConfigStore.Env().DynamoDB.Tables.User),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err = ddbClient.UpdateItem(ctx, updateItemParams)
	if err != nil {
		if repohelper.IsConditionalCheckFailedException(err) {
			return nil
		}
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
	}

	return nil
}
