package wire

import (
	"strings"
	"testing"
)

func benchFrame(binSize int) *Frame {
	return &Frame{
		MsgType:      "chat.message",
		ID:           "018f3a2b-1c4d-7e8f-9a0b-1c2d3e4f5a6b",
		ResponseToID: "018f3a2b-1c4d-7e8f-9a0b-1c2d3e4f5a6c",
		Binary:       []byte(strings.Repeat("x", binSize)),
	}
}

func BenchmarkEncode_Empty(b *testing.B) { benchEncode(b, 0) }
func BenchmarkEncode_100B(b *testing.B)  { benchEncode(b, 100) }
func BenchmarkEncode_4KB(b *testing.B)   { benchEncode(b, 4096) }

func benchEncode(b *testing.B, n int) {
	f := benchFrame(n)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Encode(f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Empty(b *testing.B) { benchDecode(b, 0) }
func BenchmarkDecode_100B(b *testing.B)  { benchDecode(b, 100) }
func BenchmarkDecode_4KB(b *testing.B)   { benchDecode(b, 4096) }

func benchDecode(b *testing.B, n int) {
	raw, err := Encode(benchFrame(n))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(raw); err != nil {
			b.Fatal(err)
		}
	}
}
