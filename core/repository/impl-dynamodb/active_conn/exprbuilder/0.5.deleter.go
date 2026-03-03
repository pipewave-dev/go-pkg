package exprbuilder

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/shared/aerror"
	"github.com/samber/lo"
)

// ActiveConnectionDeleter deletes active connections
type ActiveConnectionDeleter struct {
	ConfigStore configprovider.ConfigStore
}

// DeleteParams contains the parameters needed to delete an active connection
type DeleteParams struct {
	UserID    string
	SessionID string
}

func (deleter *ActiveConnectionDeleter) buildDeleteQuery(params *DeleteParams) *dynamodb.DeleteItemInput {
	keyAV, err := attributevalue.MarshalMap(params)
	if err != nil {
		msg := fmt.Sprintf("ActiveConnectionDeleter.Delete() failed: %v", err)
		panic(msg)
	}

	deleteParams := &dynamodb.DeleteItemInput{
		TableName: lo.ToPtr(deleter.ConfigStore.Env().DynamoDB.Tables.ActiveConnection),
		Key:       keyAV,
	}

	return deleteParams
}

func (deleter *ActiveConnectionDeleter) Delete(ctx context.Context, ddbClient *dynamodb.Client, params DeleteParams) (aErr aerror.AError) {
	deleteParams := deleter.buildDeleteQuery(&params)

	_, err := ddbClient.DeleteItem(ctx, deleteParams)
	if err != nil {
		aErr = aerror.New(ctx, aerror.ErrUnexpectedDynamoDB, err)
		return aErr
	}

	return nil
}
