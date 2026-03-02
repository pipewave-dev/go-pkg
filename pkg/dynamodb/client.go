package dynamodb

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// Client ...
func (ddb *dynamodbClient) Client() *dynamodb.Client {
	return ddb.client
}
