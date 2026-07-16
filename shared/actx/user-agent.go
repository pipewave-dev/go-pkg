package actx

func (a *aContext) SetUserAgent(userAgent string) {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	a.data.userAgent = userAgent
}
func (a *aContext) GetUserAgent() (userAgent string) {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	userAgent = a.data.userAgent
	return userAgent
}
