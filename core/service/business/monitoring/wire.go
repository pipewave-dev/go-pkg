package monitoring

import "github.com/google/wire"

// WireSet provides wire bindings for monitoring service
var WireSet = wire.NewSet(New)
