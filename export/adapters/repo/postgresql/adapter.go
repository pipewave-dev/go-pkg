package adapters

import (
	implpostgres "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres"
	"github.com/pipewave-dev/go-pkg/export/adapters"
)

var PostgresRepo adapters.RepositoryAdapter = implpostgres.NewDIPostgresRepo
