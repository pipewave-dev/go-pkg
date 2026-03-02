package dynamodb

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamodbProvider interface {
	Client() *dynamodb.Client
	ListTables(ctx context.Context) ([]string, error)
	TableInfo(ctx context.Context, tableName string) (err error)
	CreateTable(ctx context.Context, params CreateTableParams) (err error)
	CreateOrVerifyTable(ctx context.Context, params CreateTableParams) (err error)
	VerifyTable(ctx context.Context, params CreateTableParams) (err error)

	ExecuteTransactWriteItems(
		ctx context.Context,
		items ...*types.TransactWriteItem,
	) (err error)

	RecursiveBatchWriteItem(
		ctx context.Context,
		tableName string,
		reqsItems []types.WriteRequest,
		depth int) (unprocessedItems []types.WriteRequest, err error)

	/*
		RecursiveBatcGetItem

	*/
	RecursiveBatchGetItem(
		ctx context.Context,
		tableName string,
		keysAV []map[string]types.AttributeValue,
		depth int) (item []map[string]types.AttributeValue, unprocessedKeysAV []map[string]types.AttributeValue, err error)
}

type KeySchema struct {
	AttributeName string
	AttributeType types.ScalarAttributeType // S, N, B
}

type IndexSchema struct {
	IndexName    string
	PartitionKey KeySchema
	SortKey      *KeySchema // optional
}

type DynamodbConfig struct {
	Region          string
	Endpoint        *string
	Role            *string
	Profile         *string
	StaticAccessKey *string
	StaticSecretKey *string
}

type dynamodbClient struct {
	client *dynamodb.Client
	config *DynamodbConfig
}

func NewDynamoDBProvider(
	config *DynamodbConfig,
) DynamodbProvider {
	dynamodbIns := &dynamodbClient{
		config: config,
	}

	err := dynamodbIns.connect(context.TODO())
	if err != nil {
		panic(err)
	}

	return dynamodbIns
}
