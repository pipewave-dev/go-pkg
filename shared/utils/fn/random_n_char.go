package fn

import (
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"time"
)

var (
	chacha8seed = [32]byte(fmt.Appendf(nil, "%-32s", time.Now().Format(time.RFC1123)))
	randChaCha8 = rand.NewChaCha8(chacha8seed)
)

// RandNChars generates a random string with n characters.
func RandNChars(n uint) string {
	if n == 0 {
		return ""
	}

	result := make([]byte, n)
	_, _ = randChaCha8.Read(result)
	return base64.URLEncoding.EncodeToString(result)[:n]
}
