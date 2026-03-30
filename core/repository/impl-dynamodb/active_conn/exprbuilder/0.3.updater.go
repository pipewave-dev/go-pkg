package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
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

type UpdateStatusParams struct {
	UserID    string
	SessionID string
	Status    voWs.WsStatus
}

func (updater *ActiveConnectionUpdater) UpdateStatus(ctx context.Context, ddbClient *dynamodb.Client, params UpdateStatusParams) aerror.AError {
	type keySchema struct {
		UserID    string
		SessionID string
	}

	key, err := attributevalue.MarshalMap(keySchema{UserID: params.UserID, SessionID: params.SessionID})
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatus marshal key error: %v", err)
		panic(msg)
	}

	update := expression.Set(expression.Name(FieldStatus), expression.Value(params.Status))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatus build expression error: %v", err)
		panic(msg)
	}

	//nolint:exhaustruct
	input := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(updater.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err2 := ddbClient.UpdateItem(ctx, input)
	if err2 != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
	}

	return nil
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

func (updater *ActiveConnectionUpdater) UpdateStatusTransferring(ctx context.Context, ddbClient *dynamodb.Client, userID, sessionID string) aerror.AError {
	type keySchema struct {
		UserID    string
		SessionID string
	}

	key, err := attributevalue.MarshalMap(keySchema{UserID: userID, SessionID: sessionID})
	if err != nil {
		panic(fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatusTransferring marshal key error: %v", err))
	}

	update := expression.
		Set(expression.Name(FieldStatus), expression.Value(voWs.WsStatusTransferring)).
		Set(expression.Name(FieldHolderID), expression.Value(""))

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		panic(fmt.Sprintf("*ActiveConnectionUpdater.UpdateStatusTransferring build expression error: %v", err))
	}

	//nolint:exhaustruct
	input := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(updater.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}

	_, err2 := ddbClient.UpdateItem(ctx, input)
	if err2 != nil {
		return aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err2)
	}
	return nil
}
