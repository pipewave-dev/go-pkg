package actx

import (
	"log/slog"
)

func (a *aContext) SetUserIP(userIP string) {
	a.data.userIp = userIP
}
func (a *aContext) GetUserIP() (userIP string) {
	userIP = a.data.userIp
	if userIP == "" {
		slog.Error("GetUserIP fail, check logic code")
	}
	return userIP
}
