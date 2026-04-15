package dynamodb

import (
	"context"
	"fmt"

	pkgdynamodb "github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

func HandleStartupMigration(ctx context.Context, cfg configprovider.ConfigStore, dnm pkgdynamodb.DynamodbProvider, autoMigration bool) error {
	if autoMigration {
		return RunMigration(ctx, cfg, dnm)
	}
	return VerifyMigration(ctx, cfg, dnm)
}

func RunMigration(ctx context.Context, cfg configprovider.ConfigStore, dnm pkgdynamodb.DynamodbProvider) error {
	for _, table := range tablesSchema(cfg) {
		err := dnm.CreateOrVerifyTable(ctx, table)
		if err != nil {
			return fmt.Errorf("DynamoDB create or verify tables failed: %w", err)
		}
	}
	return nil
}

func VerifyMigration(ctx context.Context, cfg configprovider.ConfigStore, dnm pkgdynamodb.DynamodbProvider) error {
	for _, table := range tablesSchema(cfg) {
		err := dnm.VerifyTable(ctx, table)
		if err != nil {
			return fmt.Errorf("DynamoDB create or verify tables failed: %w", err)
		}
	}
	return nil
}
