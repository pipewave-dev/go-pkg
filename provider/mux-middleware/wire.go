package muxmiddleware

import "github.com/google/wire"

// WireSet provides wire bindings for i18n provider
var WireSet = wire.NewSet(New)
