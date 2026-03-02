package cacheprovider

import "github.com/google/wire"

// WireSet provides wire bindings for cache provider
var WireSet = wire.NewSet(New)
