package actx

func (a *aContext) SetFromBroadcast() {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	a.data.fromBroadcast = true
}

func (a *aContext) IsFromBroadcast() bool {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	return a.data.fromBroadcast
}
