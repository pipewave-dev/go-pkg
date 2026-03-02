package actx

func (a *aContext) SetFromBroadcast() {
	a.data.fromBroadcast = true
}

func (a *aContext) IsFromBroadcast() bool {
	return a.data.fromBroadcast
}
