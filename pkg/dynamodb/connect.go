package dynamodb

import (
	"context"

	awsutils "github.com/pipewave-dev/go-pkg/helper/aws-utils"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/samber/lo"
)

func (ddb *dynamodbClient) connect(ctx context.Context) (err error) {
	cfg := awsutils.CreateCredentials(
		ddb.config.Region,
		lo.FromPtr(ddb.config.Profile),
		lo.FromPtr(ddb.config.StaticAccessKey),
		lo.FromPtr(ddb.config.StaticSecretKey),
		lo.FromPtr(ddb.config.Role),
	)

	ddb.client = dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = ddb.config.Endpoint
	})

	return nil
}
