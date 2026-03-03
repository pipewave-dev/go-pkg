package gobwas

import "github.com/google/wire"

// WireSet provides wire bindings for worker pool provider
var WireSet = wire.NewSet(NewServer)
