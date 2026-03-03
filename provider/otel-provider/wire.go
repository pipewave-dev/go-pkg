package otelprovider

import "github.com/google/wire"

// WireSet provides wire bindings for otel provider
var WireSet = wire.NewSet(New)
