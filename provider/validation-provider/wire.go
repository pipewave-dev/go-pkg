package validationprovider

import "github.com/google/wire"

// WireSet provides wire bindings for validation provider
var WireSet = wire.NewSet(New)
