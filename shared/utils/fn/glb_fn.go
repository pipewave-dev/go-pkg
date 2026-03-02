package fn

import (
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/google/uuid"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/samber/lo"
)

func NewNanoID(l ...int) string {
	size := 32
	if len(l) > 0 {
		size = l[0]
	}
	id, _ := gonanoid.New(size)
	return id
}

func NewUUID() uuid.UUID {
	var (
		result uuid.UUID
		err    error
	)

	_, _, err = lo.AttemptWithDelay(3, 41*time.Millisecond, func(_ int, _ time.Duration) error {
		result, err = uuid.NewV7()
		return err
	})
	if err != nil {
		panic(err)
	}

	return result
}

func UuidToString(id uuid.UUID) string {
	var i big.Int
	i.SetBytes(id[:])
	return i.Text(62)
}

func ScanUUID(data string) (uuid.UUID, error) {
	var i big.Int
	_, ok := i.SetString(data, 62)
	if !ok {
		return uuid.Nil, fmt.Errorf("fn: invalid base62 uuid string: %s", data)
	}

	b := i.Bytes()
	if len(b) > 16 {
		return uuid.Nil, fmt.Errorf("fn: base62 value too large for uuid: %s", data)
	}

	var id uuid.UUID
	// Right-align bytes into 16-byte array (pad leading zeros)
	copy(id[16-len(b):], b)
	return id, nil
}

func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[Random.Intn(len(charset))]
	}
	return string(b)
}

var (
	// seed random
	randSeed = rand.NewSource(time.Now().UnixNano())
	Random   = rand.New(randSeed) //nolint:gosec // standard library is strong enough
)
