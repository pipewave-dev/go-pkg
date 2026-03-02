package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	"github.com/pipewave-dev/go-pkg/global/constants"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

// ActiveConnectionUpdater updates the LastHeartbeat and TTL of an active connection
type ActiveConnectionUpdater struct {
	ConfigStore configprovider.ConfigStore
}

type UpdateLastHeartbeatParams struct {
	UserID    string
	SessionID string
}

func (updater *ActiveConnectionUpdater) buildUpdateHeartbeatQuery(params UpdateLastHeartbeatParams, now time.Time) *dynamodb.UpdateItemInput {
	key, err := attributevalue.MarshalMap(params)
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionUpdater marshal key error: %v", err)
		panic(msg)
	}

	ttl := now.Add(2*constants.GlobalHeartbeatRateDuration + time.Second)

	update := expression.
		Set(expression.Name(FieldLastHeartbeat), expression.Value(voUnixTime.UnixMilliTime(now))).
		Set(expression.Name(FieldTTL), expression.Value(voUnixTime.UnixMilliTime(ttl)))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionUpdater build expression error: %v", err)
		panic(msg)
	}

	//nolint:exhaustruct
	return &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(updater.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}
}

func (updater *ActiveConnectionUpdater) UpdateLastHeartbeat(ctx context.Context, ddbClient *dynamodb.Client, params UpdateLastHeartbeatParams) aerror.AError {
	now := time.Now()

	updateInput := updater.buildUpdateHeartbeatQuery(params, now)

	_, err := ddbClient.UpdateItem(ctx, updateInput)
	if err != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
	}

	return nil
}
