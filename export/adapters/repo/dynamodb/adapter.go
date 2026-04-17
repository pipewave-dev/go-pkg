package adapters

import (
	impldynamodb "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb"
	"github.com/pipewave-dev/go-pkg/export/adapters"
)

var DynamoRepo adapters.RepositoryAdapter = impldynamodb.NewDIDynamoDBRepo
