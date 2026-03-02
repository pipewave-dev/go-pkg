package observerprovider

import "github.com/google/wire"

// WireSet provides wire bindings for observer provider
var WireSet = wire.NewSet(New)
