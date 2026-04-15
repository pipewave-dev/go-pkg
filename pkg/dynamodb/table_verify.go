package dynamodb

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const fnNameVerifyTable = "(*dynamodbClient).VerifyTable"

// VerifyTable verify a DynamoDB table has same property as CreateTableParams
func (ddb *dynamodbClient) VerifyTable(ctx context.Context, params CreateTableParams) (err error) {
	tableName := params.TableName

	output, err := ddb.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return fmt.Errorf("%s: failed to describe table %s: %w", fnNameVerifyTable, tableName, err)
	}

	table := output.Table

	// 1. Verify Primary Key Schema
	if !isKeySchemaMatched(table.KeySchema, params.PartitionKey, params.SortKey) {
		return fmt.Errorf("%s: primary key schema mismatch for table %s", fnNameVerifyTable, tableName)
	}

	// 2. Verify GSIs, allow more GSIs exist as long as expected GSIs are present
	if len(table.GlobalSecondaryIndexes) < len(params.GSIs) {
		return fmt.Errorf("%s: GSI count mismatch for table %s: found %d, expected %d", fnNameVerifyTable, tableName, len(table.GlobalSecondaryIndexes), len(params.GSIs))
	}

	for _, expectedGSI := range params.GSIs {
		foundGSI := false
		for _, actualGSI := range table.GlobalSecondaryIndexes {
			if aws.ToString(actualGSI.IndexName) == expectedGSI.IndexName {
				if !isKeySchemaMatched(actualGSI.KeySchema, expectedGSI.PartitionKey, expectedGSI.SortKey) {
					return fmt.Errorf("%s: GSI %s key schema mismatch", fnNameVerifyTable, expectedGSI.IndexName)
				}
				foundGSI = true
				break
			}
		}
		if !foundGSI {
			return fmt.Errorf("%s: GSI %s not found", fnNameVerifyTable, expectedGSI.IndexName)
		}
	}

	return nil
}

func isKeySchemaMatched(actual []types.KeySchemaElement, pk KeySchema, sk *KeySchema) bool {
	expectedLen := 1
	if sk != nil {
		expectedLen = 2
	}

	if len(actual) != expectedLen {
		return false
	}

	// Check Partition Key
	pkFound := false
	for _, k := range actual {
		if k.KeyType == types.KeyTypeHash && aws.ToString(k.AttributeName) == pk.AttributeName {
			pkFound = true
			break
		}
	}
	if !pkFound {
		return false
	}

	// Check Sort Key
	if sk != nil {
		skFound := false
		for _, k := range actual {
			if k.KeyType == types.KeyTypeRange && aws.ToString(k.AttributeName) == sk.AttributeName {
				skFound = true
				break
			}
		}
		if !skFound {
			return false
		}
	}

	return true
}
