package healthyprovider

import "github.com/google/wire"

// WireSet provides wire bindings for healthy provider
var WireSet = wire.NewSet(New)
