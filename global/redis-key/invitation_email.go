package rediskey

import "fmt"

func GenerateInvitationKey(email string) string {
	return fmt.Sprintf("invitation-%s", email)
}
