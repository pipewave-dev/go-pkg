package dynamodb

import (
	"context"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (dynamodb.DynamodbProvider, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	dynamodbPrv := dynamodb.NewDynamoDBProvider(dynamoDBConfig(c))
	if err := HandleStartupMigration(context.Background(), c, dynamodbPrv, c.Env().AutoMigration); err != nil {
		return nil, err
	}
	return dynamodbPrv, nil
}

func New(cfg configprovider.ConfigStore) dynamodb.DynamodbProvider {
	dynamodbPrv := dynamodb.NewDynamoDBProvider(dynamoDBConfig(cfg))
	if err := HandleStartupMigration(context.Background(), cfg, dynamodbPrv, cfg.Env().AutoMigration); err != nil {
		panic(err.Error())
	}
	return dynamodbPrv
}

func GetClient(dnm dynamodb.DynamodbProvider) *ddb.Client {
	return dnm.Client()
}

func dynamoDBConfig(cfg configprovider.ConfigStore) *dynamodb.DynamodbConfig {
	ddbCfg := cfg.Env().DynamoDB
	ddbCfn := dynamodb.DynamodbConfig{
		Region:          ddbCfg.Region,
		Endpoint:        ddbCfg.Endpoint,
		Role:            ddbCfg.Role,
		StaticAccessKey: ddbCfg.StaticAccessKey,
		StaticSecretKey: ddbCfg.StaticSecretKey,
	}
	return &ddbCfn
}

func tablesSchema(cfg configprovider.ConfigStore) []dynamodb.CreateTableParams {
	tables := cfg.Env().DynamoDB.Tables
	return []dynamodb.CreateTableParams{
		// active_connection
		// PK: UserID (S), SK: InstanceID (S)
		// Queries: CountActive by UserID + filter on LastHeartbeat (no GSI needed)
		{
			TableName: tables.ActiveConnection,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "UserID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "InstanceID",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// fcm_device
		// PK: UserID (S), SK: InstanceID (S)
		// Queries:
		//   - ByUser: query main table by UserID (PK)
		//   - ByToken: query GSI FcmDeviceTokenIndex by FcmDeviceToken
		//   - ByUserSession: query main table by UserID + InstanceID (PK+SK)
		{
			TableName: tables.FcmDevice,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "UserID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "InstanceID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			GSIs: []dynamodb.IndexSchema{
				{
					// GSI-1: query FcmDevice by FcmDeviceToken (unique per device)
					IndexName: "FcmDeviceTokenIndex",
					PartitionKey: dynamodb.KeySchema{
						AttributeName: "FcmDeviceToken",
						AttributeType: types.ScalarAttributeTypeS,
					},
				},
			},
		},
		// group
		// PK: ID (S), no SK
		// Queries: ByID (GetItem / Query by PK)
		{
			TableName: tables.Group,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "ID",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// user
		// PK: ID (S), no SK
		// Queries: ByID (Query by PK)
		{
			TableName: tables.User,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "ID",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// user_group
		// PK: GroupID (S), SK: UserID (S)
		// Queries:
		//   - ByPrimaryKey: query main table by GroupID + UserID
		//   - ListUsersByGroup: query main table by GroupID (PK)
		//   - ListGroupsByUser: query GSI UserIDGroupIDIndex by UserID
		{
			TableName: tables.UserGroup,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "GroupID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "UserID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			GSIs: []dynamodb.IndexSchema{
				{
					// GSI-1: query UserGroup by UserID to list all groups a user belongs to
					IndexName: "UserIDGroupIDIndex",
					PartitionKey: dynamodb.KeySchema{
						AttributeName: "UserID",
						AttributeType: types.ScalarAttributeTypeS,
					},
					SortKey: &dynamodb.KeySchema{
						AttributeName: "GroupID",
						AttributeType: types.ScalarAttributeTypeS,
					},
				},
			},
		},
		// noti_content
		// PK: ID (S), no SK
		// Queries: ByID (Query by PK), ByIDs (BatchGetItem by IDs)
		{
			TableName: tables.NotiContent,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "ID",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// noti_time_bucket
		// PK: ReceiverID (S), SK: ReceiverType (N)
		// ReceiverType: 1=Group, 2=User
		// Queries: GetByGroupID / GetByUserID (GetItem by ReceiverID + ReceiverType)
		{
			TableName: tables.NotiTimeBucket,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "ReceiverID",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "ReceiverType",
				AttributeType: types.ScalarAttributeTypeN,
			},
		},
		// g_noti (Group Notification)
		// PK: PartitionKey (S) — composite: "{ReceiverId}#{TimeBucket}#{TimeBucketRule}"
		// SK: SortKey (S)      — composite: "{ReceiveAt_base36}#{MessageId}"
		// Queries:
		//   - ByKey: GetItem by full PK+SK
		//   - ListAfter: Query PK + SK > lowerBound (forward scan across TimeBuckets)
		//   - ListBefore: Query PK + SK < upperBound (backward scan across TimeBuckets)
		{
			TableName: tables.GNoti,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "PartitionKey",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "SortKey",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// u_noti (User Notification)
		// PK: PartitionKey (S) — composite: "{UserID}#{TimeBucket}#{TimeBucketRule}"
		// SK: SortKey (S)      — composite: "{ReceiveAt_base36}#{MessageId}"
		// Queries:
		//   - ByKey: GetItem by full PK+SK
		//   - ListAfter: Query PK + SK > lowerBound (forward scan across TimeBuckets)
		//   - ListBefore: Query PK + SK < upperBound (backward scan across TimeBuckets)
		{
			TableName: tables.UNoti,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "PartitionKey",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "SortKey",
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		// pending_message
		// PK: SessionKey (S) — composite: "{userID}:{instanceID}"
		// SK: SendAt (N)     — Unix nano int64, ascending order for GetAll
		// TTL: TTL           — same duration as MessageHub.TTL (temp-disconnect window)
		{
			TableName: tables.PendingMessage,
			PartitionKey: dynamodb.KeySchema{
				AttributeName: "SessionKey",
				AttributeType: types.ScalarAttributeTypeS,
			},
			SortKey: &dynamodb.KeySchema{
				AttributeName: "SendAt",
				AttributeType: types.ScalarAttributeTypeN,
			},
		},
	}
}
