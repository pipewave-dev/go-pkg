package exprbuilder

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/domain/entities"
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
	UserID    string
	SessionID string

	HolderID       string
	ConnectionType voWs.WsCoreType
}

func (creator *ActiveConnectionCreator) Create(ctx context.Context, ddbClient *dynamodb.Client, params CreateParams) (*entities.ActiveConnection, aerror.AError) {
	now := time.Now()
	result := &entities.ActiveConnection{
		UserID:    params.UserID,
		SessionID: params.SessionID,
		HolderID:  params.HolderID,

		ConnectionType: params.ConnectionType,
		Status:         voWs.WsStatusConnected,
		ConnectedAt:    now,
		LastHeartbeat:  now,
		TTL:            now.Add(2*constants.GlobalHeartbeatRateDuration + time.Second),
	}

	activeConnectionDataAV, err := toDynamoMap(result)
	if err != nil {
		msg := fmt.Sprintf("*ActiveConnectionCreator unmarshal error: %v", err)
		panic(msg)
	}

	//nolint:exhaustruct
	putItemParams := &dynamodb.PutItemInput{
		TableName: lo.ToPtr(creator.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Item:      activeConnectionDataAV,
	}

	_, err = ddbClient.PutItem(ctx, putItemParams)
	if err != nil {
		aErr := aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return nil, aErr
	}

	return result, nil
}
