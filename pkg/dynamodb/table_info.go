package dynamodb

import (
	"bytes"
	"context"
	"log/slog"

	"github.com/goccy/go-json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// TableInfo print to log a detail of the table (using for debug).
func (ddb *dynamodbClient) TableInfo(
	ctx context.Context,
	tableName string,
) (err error) {
	output, err := ddb.client.DescribeTable(
		ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)},
	)
	if err != nil {
		slog.ErrorContext(ctx, "(*dynamodbClient).TableInfo",
			slog.Any("err", err))
	}
	detail := output.Table
	slog.InfoContext(ctx, "(*dynamodbClient).TableInfo",
		slog.String("tableName", tableName),
		slog.String("detail", prettyJSON(detail)))
	return nil
}

func prettyJSON(body any) string {
	var prettyJSON bytes.Buffer
	b, _ := json.Marshal(body)
	if err := json.Indent(&prettyJSON, b, "", "\t"); err != nil {
		return ""
	}
	return prettyJSON.String()
}
