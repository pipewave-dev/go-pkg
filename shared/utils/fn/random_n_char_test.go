package fn

import (
	"testing"
)

func TestRandNChars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		n       uint
		wantLen int
		checkFn func(t *testing.T, result string)
	}{
		{
			name:    "zero length returns empty string",
			n:       0,
			wantLen: 0,
			checkFn: func(t *testing.T, result string) {
				if result != "" {
					t.Errorf("expected empty string, got %q", result)
				}
			},
		},
		{
			name:    "single character",
			n:       1,
			wantLen: 1,
			checkFn: nil,
		},
		{
			name:    "short string",
			n:       10,
			wantLen: 10,
			checkFn: nil,
		},
		{
			name:    "medium string",
			n:       100,
			wantLen: 100,
			checkFn: nil,
		},
		{
			name:    "long string",
			n:       1000,
			wantLen: 1000,
			checkFn: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RandNChars(tt.n)

			// Check length
			if len(result) != tt.wantLen {
				t.Errorf("RandNChars(%d) returned length %d, want %d", tt.n, len(result), tt.wantLen)
			}

			// Run custom check function if provided
			if tt.checkFn != nil {
				tt.checkFn(t, result)
			}
		})
	}
}

func TestRandNChars_Randomness(t *testing.T) {
	t.Parallel()

	// Generate multiple strings and ensure they're different
	iterations := 100
	n := uint(20)
	results := make(map[string]bool)

	for i := 0; i < iterations; i++ {
		result := RandNChars(n)
		results[result] = true
	}

	// With a good random generator, we should get mostly unique results
	// We'll accept at least 95% uniqueness for 100 iterations of 20 byte strings
	minUnique := 95
	uniqueCount := len(results)
	if uniqueCount < minUnique {
		t.Errorf("Expected at least %d unique results out of %d, got %d", minUnique, iterations, uniqueCount)
	}
}

func TestRandNChars_Consistency(t *testing.T) {
	t.Parallel()

	// Verify that the function consistently returns the correct length
	sizes := []uint{0, 1, 5, 10, 50, 100, 256, 512, 1024}

	for _, size := range sizes {
		size := size
		t.Run(string(rune(size)), func(t *testing.T) {
			t.Parallel()

			for i := 0; i < 10; i++ {
				result := RandNChars(size)
				if uint(len(result)) != size {
					t.Errorf("iteration %d: RandNChars(%d) returned length %d", i, size, len(result))
				}
			}
		})
	}
}

func BenchmarkRandNChars(b *testing.B) {
	benchmarks := []struct {
		name string
		n    uint
	}{
		{"1byte", 1},
		{"10bytes", 10},
		{"100bytes", 100},
		{"1kb", 1024},
		{"10kb", 10240},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = RandNChars(bm.n)
			}
		})
	}
}
