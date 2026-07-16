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
	voUnixTime "github.com/pipewave-dev/go-pkg/core/domain/value-object/unixtime"
	voWs "github.com/pipewave-dev/go-pkg/core/domain/value-object/ws"
	"github.com/pipewave-dev/go-pkg/global/constants"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

type ActiveConnectionCreator struct {
	ConfigStore configprovider.ConfigStore
}

type CreateParams struct {
	UserID     string
	InstanceID string

	HolderID       string
	ConnectionType voWs.WsCoreType
}

func (creator *ActiveConnectionCreator) Create(ctx context.Context, ddbClient *dynamodb.Client, params CreateParams) (*entities.ActiveConnection, aerror.AError) {
	now := time.Now()
	nowMilli := voUnixTime.UnixMilliTime(now)
	ttlMilli := voUnixTime.UnixMilliTime(now.Add(2*constants.GlobalHeartbeatRateDuration + time.Second))

	key, err := attributevalue.MarshalMap(struct {
		UserID     string
		InstanceID string
	}{UserID: params.UserID, InstanceID: params.InstanceID})
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionCreator marshal key error: %v", err)
		panic(msg)
	}

	// ConnectedAt is only set on first connect: on reconnect (same UserID +
	// InstanceID), IfNotExists keeps the value already stored instead of
	// resetting it to now, matching the Postgres add_connection.go behavior.
	update := expression.
		Set(expression.Name(FieldHolderID), expression.Value(params.HolderID)).
		Set(expression.Name(FieldConnectionType), expression.Value(params.ConnectionType)).
		Set(expression.Name(FieldStatus), expression.Value(voWs.WsStatusConnected)).
		Set(expression.Name(FieldConnectedAt), expression.IfNotExists(expression.Name(FieldConnectedAt), expression.Value(nowMilli))).
		Set(expression.Name(FieldLastHeartbeat), expression.Value(nowMilli)).
		Set(expression.Name(FieldTTL), expression.Value(ttlMilli))

	expr, errB := expression.NewBuilder().WithUpdate(update).Build()
	if errB != nil {
		msg := fmt.Sprintf("*ActiveConnectionCreator build expression error: %v", errB)
		panic(msg)
	}

	//nolint:exhaustruct
	updateItemParams := &dynamodb.UpdateItemInput{
		TableName:                 lo.ToPtr(creator.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:                       key,
		UpdateExpression:          expr.Update(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ReturnValues:              types.ReturnValueAllNew,
	}

	output, err := ddbClient.UpdateItem(ctx, updateItemParams)
	if err != nil {
		aErr := aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return nil, aErr
	}

	result, err := fromDynamoMap(output.Attributes)
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionCreator unmarshal error: %v", err)
		panic(msg)
	}

	return result, nil
}
