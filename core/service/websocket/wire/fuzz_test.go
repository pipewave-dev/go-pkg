package wire

import "testing"

// FuzzDecode asserts Decode never panics on arbitrary input. Every frame
// reaching the handler is attacker-controlled, so a panic here is a
// remotely-triggerable crash.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{0x01, 0x00, 0xCA})
	f.Add([]byte{0x01, 0x02, 0x68, 0x69})
	f.Add([]byte{0xf1, 0x01, 0x74, 0x01, 0x69, 0x01, 0x72, 0x01, 0x61, 0x00, 0x01, 0x65, 0xff})
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := Decode(data)
		if err != nil {
			return
		}
		// A frame that decoded must re-encode without error.
		if _, err := Encode(frame); err != nil {
			t.Fatalf("decoded frame failed to re-encode: %v", err)
		}
	})
}
