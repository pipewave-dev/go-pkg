package fn

import (
	"math"
	"math/rand/v2"
	"time"
)

var (
	random = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
)

// RandIntNDigits generates a random integer with n digits.
func RandIntNDigits(n uint) uint {
	lowestNumberNDigits := uint(math.Pow(10, float64(n-1)))

	if n <= 0 {
		return lowestNumberNDigits
	}

	highestNumberNDigits := uint(math.Pow(10, float64(n))) - 1

	return random.UintN(highestNumberNDigits-
		lowestNumberNDigits,
	) + lowestNumberNDigits
}
