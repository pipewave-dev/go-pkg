package actx

func (a *aContext) SetUserAgent(userAgent string) {
	a.data.userAgent = userAgent
}
func (a *aContext) GetUserAgent() (userAgent string) {
	userAgent = a.data.userAgent
	return userAgent
}
