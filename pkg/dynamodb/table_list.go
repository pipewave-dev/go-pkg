package dynamodb

import (
	"context"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const fnNameListTables = "(*dynamodbClient).ListTables"

// ListTables returns a list of table names.
func (ddb *dynamodbClient) ListTables(ctx context.Context) (tables []string, err error) {
	var tableNames []string
	output, err := ddb.client.ListTables(
		ctx, &dynamodb.ListTablesInput{})
	if err != nil {
		slog.ErrorContext(ctx, fnNameListTables+" #1",
			slog.Any("err", err))
	} else {
		tableNames = output.TableNames
	}
	return tableNames, err
}
