package adapters

import (
	"github.com/pipewave-dev/go-pkg/export/adapters"
	queue "github.com/pipewave-dev/go-pkg/provider/queue/valkey"
)

var QueueValkey adapters.QueueAdapter = queue.QueueValkeyDI
