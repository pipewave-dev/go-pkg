package dynamodb

import (
	"bytes"
	"context"
	"fmt"
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
	fmt.Printf("TableInfo: %s", tableName)
	prettyJSON(detail)
	return nil
}

func prettyJSON(body any) {
	var prettyJSON bytes.Buffer
	b, _ := json.Marshal(body)
	err := json.Indent(&prettyJSON, b, "", "\t")
	if err != nil {
		return
	}
	fmt.Println(prettyJSON.String())
}
