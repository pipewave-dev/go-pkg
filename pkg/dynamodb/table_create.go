package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const fnNameCreateTable = "(*dynamodbClient).CreateTable"

type CreateTableParams struct {
	TableName    string
	PartitionKey KeySchema
	SortKey      *KeySchema    // optional
	GSIs         []IndexSchema // optional
}

// CreateTable creates a new DynamoDB table.
func (ddb *dynamodbClient) CreateTable(ctx context.Context, params CreateTableParams) (err error) {
	tableName := params.TableName
	attrMap := make(map[string]types.ScalarAttributeType)

	// Primary Key Attributes
	if err := addAttributeToMap(attrMap, params.PartitionKey); err != nil {
		return err
	}
	if params.SortKey != nil {
		if err := addAttributeToMap(attrMap, *params.SortKey); err != nil {
			return err
		}
	}

	// GSI Attributes and Index Definitions
	var gsis []types.GlobalSecondaryIndex
	for _, gsi := range params.GSIs {
		if err := addAttributeToMap(attrMap, gsi.PartitionKey); err != nil {
			return err
		}
		if gsi.SortKey != nil {
			if err := addAttributeToMap(attrMap, *gsi.SortKey); err != nil {
				return err
			}
		}

		gsis = append(gsis, types.GlobalSecondaryIndex{
			IndexName: aws.String(gsi.IndexName),
			KeySchema: toKeySchema(gsi.PartitionKey, gsi.SortKey),
			Projection: &types.Projection{
				ProjectionType: types.ProjectionTypeAll,
			},
		})
	}

	input := &dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		AttributeDefinitions: toAttributeDefinitions(attrMap),
		KeySchema:            toKeySchema(params.PartitionKey, params.SortKey),
		BillingMode:          types.BillingModePayPerRequest,
	}

	if len(gsis) > 0 {
		input.GlobalSecondaryIndexes = gsis
	}

	_, err = ddb.client.CreateTable(ctx, input)
	if err != nil {
		slog.ErrorContext(ctx, fnNameCreateTable+" #1",
			slog.String("tableName", tableName),
			slog.Any("err", err))
		return err
	}

	return nil
}

// CreateOrVerifyTable creates a new DynamoDB table.
func (ddb *dynamodbClient) CreateOrVerifyTable(ctx context.Context, params CreateTableParams) (err error) {
	err = ddb.VerifyTable(ctx, params)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return ddb.CreateTable(ctx, params)
		}
		return err
	}
	return nil
}

func addAttributeToMap(attrMap map[string]types.ScalarAttributeType, k KeySchema) error {
	if oldAttrType, ok := attrMap[k.AttributeName]; ok {
		if oldAttrType != k.AttributeType {
			return fmt.Errorf("Duplicated attribute name but difference type: %s", k.AttributeName)
		}
	}
	attrMap[k.AttributeName] = k.AttributeType
	return nil
}

func toAttributeDefinitions(attrMap map[string]types.ScalarAttributeType) []types.AttributeDefinition {
	attrDefinitions := make([]types.AttributeDefinition, 0, len(attrMap))
	for name, attrType := range attrMap {
		attrDefinitions = append(attrDefinitions, types.AttributeDefinition{
			AttributeName: aws.String(name),
			AttributeType: attrType,
		})
	}
	return attrDefinitions
}

func toKeySchema(partitionKey KeySchema, sortKey *KeySchema) []types.KeySchemaElement {
	keySchema := []types.KeySchemaElement{
		{
			AttributeName: aws.String(partitionKey.AttributeName),
			KeyType:       types.KeyTypeHash,
		},
	}
	if sortKey != nil {
		keySchema = append(keySchema, types.KeySchemaElement{
			AttributeName: aws.String(sortKey.AttributeName),
			KeyType:       types.KeyTypeRange,
		})
	}
	return keySchema
}
