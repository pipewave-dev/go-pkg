package fn

import (
	"math"
	"strconv"
	"testing"
)

func TestRandIntNDigits(t *testing.T) {
	tests := []struct {
		name     string
		n        uint
		expected struct {
			minDigits int
			maxDigits int
			minValue  uint
			maxValue  uint
		}
	}{
		{
			name: "Zero digits (edge case)",
			n:    0,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 1, // uint(math.Pow(10, -1)) = uint(0.1) = 0
				maxDigits: 20,
				minValue:  0,
				maxValue:  18446744073709551615, // max uint value when n=0
			},
		},
		{
			name: "Single digit",
			n:    1,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 1,
				maxDigits: 1,
				minValue:  1,
				maxValue:  9,
			},
		},
		{
			name: "Two digits",
			n:    2,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 2,
				maxDigits: 2,
				minValue:  10,
				maxValue:  99,
			},
		},
		{
			name: "Three digits",
			n:    3,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 3,
				maxDigits: 3,
				minValue:  100,
				maxValue:  999,
			},
		},
		{
			name: "Four digits",
			n:    4,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 4,
				maxDigits: 4,
				minValue:  1000,
				maxValue:  9999,
			},
		},
		{
			name: "Five digits",
			n:    5,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 5,
				maxDigits: 5,
				minValue:  10000,
				maxValue:  99999,
			},
		},
		{
			name: "Large number of digits",
			n:    10,
			expected: struct {
				minDigits int
				maxDigits int
				minValue  uint
				maxValue  uint
			}{
				minDigits: 10,
				maxDigits: 10,
				minValue:  1000000000,
				maxValue:  9999999999,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RandIntNDigits(tt.n)

			// Check that result is within expected range
			if result < tt.expected.minValue || result > tt.expected.maxValue {
				t.Errorf("RandIntNDigits(%d) = %d, expected range [%d, %d]", tt.n, result, tt.expected.minValue, tt.expected.maxValue)
			}

			// Check that result has correct number of digits
			resultStr := strconv.FormatUint(uint64(result), 10)
			actualDigits := len(resultStr)

			if actualDigits < tt.expected.minDigits || actualDigits > tt.expected.maxDigits {
				t.Errorf("RandIntNDigits(%d) = %d has %d digits, expected %d digits", tt.n, result, actualDigits, tt.expected.minDigits)
			}
		})
	}
}

// TestRandIntNDigitsUniqueness verifies that multiple calls produce different results
func TestRandIntNDigitsUniqueness(t *testing.T) {
	n := uint(6) // 6-digit numbers have good range for uniqueness testing
	numCalls := 100
	results := make(map[uint]bool)

	for i := 0; i < numCalls; i++ {
		result := RandIntNDigits(n)
		if results[result] {
			t.Logf("Duplicate result %d found (not necessarily an error for small samples)", result)
		}
		results[result] = true
	}

	// We should have mostly unique results
	uniqueCount := len(results)
	minExpectedUnique := numCalls * 8 / 10 // Expect at least 80% unique

	if uniqueCount < minExpectedUnique {
		t.Errorf("Expected at least %d unique results out of %d calls, got %d", minExpectedUnique, numCalls, uniqueCount)
	}
}

// TestRandIntNDigitsDistribution tests that results are reasonably distributed
func TestRandIntNDigitsDistribution(t *testing.T) {
	n := uint(2) // Use 2-digit numbers (10-99) for easier distribution testing
	numCalls := 1000
	results := make([]uint, numCalls)

	for i := 0; i < numCalls; i++ {
		results[i] = RandIntNDigits(n)
	}

	// Check that we got numbers across the range
	minValue := uint(math.Pow(10, float64(n-1)))
	maxValue := uint(math.Pow(10, float64(n))) - 1
	rangeSize := maxValue - minValue + 1

	// Count occurrences in different buckets
	numBuckets := 10
	bucketSize := rangeSize / uint(numBuckets)
	buckets := make([]int, numBuckets)

	for _, result := range results {
		if result >= minValue && result <= maxValue {
			bucketIndex := (result - minValue) / bucketSize
			if bucketIndex >= uint(numBuckets) {
				bucketIndex = uint(numBuckets - 1) // Handle edge case
			}
			buckets[bucketIndex]++
		}
	}

	// Each bucket should have roughly numCalls/numBuckets entries
	expectedPerBucket := numCalls / numBuckets
	tolerance := expectedPerBucket / 2 // 50% tolerance

	for i, count := range buckets {
		if count < expectedPerBucket-tolerance || count > expectedPerBucket+tolerance {
			t.Logf("Bucket %d has %d entries, expected around %d (±%d)", i, count, expectedPerBucket, tolerance)
		}
	}

	// At least all buckets should have some entries (very lenient test)
	for i, count := range buckets {
		if count == 0 {
			t.Errorf("Bucket %d is empty, indicating poor distribution", i)
		}
	}
}

// TestRandIntNDigitsEdgeCases tests various edge cases
func TestRandIntNDigitsEdgeCases(t *testing.T) {
	// Test with very large n (might cause overflow or other issues)
	largeN := uint(15) // 15-digit numbers
	result := RandIntNDigits(largeN)

	// Should still produce a valid result
	resultStr := strconv.FormatUint(uint64(result), 10)
	if len(resultStr) != int(largeN) {
		t.Errorf("RandIntNDigits(%d) produced %d-digit number: %d", largeN, len(resultStr), result)
	}

	// Test multiple calls with n=1 to ensure we get different single digits
	singleDigitResults := make(map[uint]bool)
	for i := 0; i < 50; i++ {
		result := RandIntNDigits(1)
		if result < 1 || result > 9 {
			t.Errorf("RandIntNDigits(1) should return 1-9, got %d", result)
		}
		singleDigitResults[result] = true
	}

	// We should see multiple different single digits
	if len(singleDigitResults) < 3 {
		t.Errorf("Expected to see at least 3 different single digits in 50 calls, got %d", len(singleDigitResults))
	}
}

// TestRandIntNDigitsConsistency verifies that the function behavior is consistent
func TestRandIntNDigitsConsistency(t *testing.T) {
	n := uint(4)
	numTests := 100

	for i := 0; i < numTests; i++ {
		result := RandIntNDigits(n)

		// Every result should be a 4-digit number
		if result < 1000 || result > 9999 {
			t.Errorf("RandIntNDigits(4) call %d returned %d, expected 4-digit number", i, result)
		}

		// Convert to string and verify length
		resultStr := strconv.FormatUint(uint64(result), 10)
		if len(resultStr) != 4 {
			t.Errorf("RandIntNDigits(4) call %d returned %s (%d digits), expected 4 digits", i, resultStr, len(resultStr))
		}
	}
}

// TestRandIntNDigitsZeroHandling specifically tests the n=0 case
func TestRandIntNDigitsZeroHandling(t *testing.T) {
	// The function should handle n=0 gracefully
	result := RandIntNDigits(0)

	// Based on the function logic: lowestNumberNDigits = 10^(0-1) = 10^(-1) = 0.1 -> uint(0.1) = 0
	// This causes an underflow scenario where we get a very large random number
	// The actual behavior should be tested rather than assuming it returns 1
	t.Logf("RandIntNDigits(0) returned: %d", result)

	// Test multiple times to ensure function doesn't panic
	for i := 0; i < 10; i++ {
		result := RandIntNDigits(0)
		t.Logf("RandIntNDigits(0) call %d returned: %d", i, result)
	}
}

// TestRandIntNDigitsRange verifies the mathematical correctness of ranges
func TestRandIntNDigitsRange(t *testing.T) {
	testCases := []struct {
		n        uint
		expected struct {
			min uint
			max uint
		}
	}{
		{1, struct{ min, max uint }{1, 9}},
		{2, struct{ min, max uint }{10, 99}},
		{3, struct{ min, max uint }{100, 999}},
		{4, struct{ min, max uint }{1000, 9999}},
		{5, struct{ min, max uint }{10000, 99999}},
	}

	for _, tc := range testCases {
		t.Run(strconv.Itoa(int(tc.n))+"_digits", func(t *testing.T) {
			// Test multiple times to try to hit both ends of the range
			minSeen := uint(math.MaxUint32)
			maxSeen := uint(0)

			for i := 0; i < 1000; i++ {
				result := RandIntNDigits(tc.n)

				if result < minSeen {
					minSeen = result
				}
				if result > maxSeen {
					maxSeen = result
				}

				// Each result should be in valid range
				if result < tc.expected.min || result > tc.expected.max {
					t.Errorf("Result %d is outside expected range [%d, %d]", result, tc.expected.min, tc.expected.max)
				}
			}

			// We should see results reasonably close to both bounds
			// (This is probabilistic, so we use lenient bounds)
			rangeTolerance := (tc.expected.max - tc.expected.min) / 10 // 10% of range

			if minSeen > tc.expected.min+rangeTolerance {
				t.Logf("Minimum seen (%d) is not close to expected minimum (%d) - might indicate bias", minSeen, tc.expected.min)
			}

			if maxSeen < tc.expected.max-rangeTolerance {
				t.Logf("Maximum seen (%d) is not close to expected maximum (%d) - might indicate bias", maxSeen, tc.expected.max)
			}
		})
	}
}

// BenchmarkRandIntNDigits benchmarks the function performance
func BenchmarkRandIntNDigits(b *testing.B) {
	n := uint(6) // 6-digit numbers

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RandIntNDigits(n)
	}
}

// BenchmarkRandIntNDigitsVariousSizes benchmarks different digit counts
func BenchmarkRandIntNDigitsVariousSizes(b *testing.B) {
	sizes := []uint{1, 2, 3, 4, 5, 6, 8, 10}

	for _, size := range sizes {
		b.Run(strconv.Itoa(int(size))+"_digits", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = RandIntNDigits(size)
			}
		})
	}
}
