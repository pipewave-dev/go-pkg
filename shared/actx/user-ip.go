package actx

import (
	"log/slog"
)

func (a *aContext) SetUserIP(userIP string) {
	a.data.m.Lock()
	defer a.data.m.Unlock()
	a.data.userIp = userIP
}
func (a *aContext) GetUserIP() (userIP string) {
	a.data.m.Lock()
	userIP = a.data.userIp
	a.data.m.Unlock()
	if userIP == "" {
		slog.Error("GetUserIP fail, check logic code")
	}
	return userIP
}
