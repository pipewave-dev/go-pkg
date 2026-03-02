package pubsubprovider

import "github.com/google/wire"

// WireSet provides wire bindings for pubsub provider
var WireSet = wire.NewSet(New)
