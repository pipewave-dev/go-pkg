package adapters

import (
	"github.com/pipewave-dev/go-pkg/export/adapters"
	pubsub "github.com/pipewave-dev/go-pkg/provider/pubsub/valkey"
)

var PubsubValkey adapters.PubsubAdapter = pubsub.PubsubValkeyDI
