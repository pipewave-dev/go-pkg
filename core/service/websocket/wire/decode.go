package wire

// Decode parses a v1 frame. Every read is bounds-checked before slicing:
// input arrives from untrusted WebSocket clients, so an out-of-range slice
// would be a remotely-triggerable panic.
//
// The returned Frame's Binary aliases data; callers must not mutate it.
func Decode(data []byte) (*Frame, error) {
	if len(data) < 2 {
		return nil, ErrMalformed
	}
	if data[0]&0x0F != Version {
		return nil, ErrVersion
	}
	flags := data[0] & 0xF0

	f := &Frame{}
	pos := 1

	msgTypeLen := int(data[pos])
	pos++
	if msgTypeLen == 0 {
		if pos >= len(data) {
			return nil, ErrMalformed
		}
		f.ControlCode = data[pos]
		pos++
	} else {
		if pos+msgTypeLen > len(data) {
			return nil, ErrMalformed
		}
		f.MsgType = string(data[pos : pos+msgTypeLen])
		pos += msgTypeLen
	}

	readShort := func() (string, bool) {
		if pos >= len(data) {
			return "", false
		}
		n := int(data[pos])
		pos++
		if pos+n > len(data) {
			return "", false
		}
		s := string(data[pos : pos+n])
		pos += n
		return s, true
	}

	// Order must match the encoder's flag-bit order.
	if flags&FlagID != 0 {
		s, ok := readShort()
		if !ok {
			return nil, ErrMalformed
		}
		f.ID = s
	}
	if flags&FlagResponseToID != 0 {
		s, ok := readShort()
		if !ok {
			return nil, ErrMalformed
		}
		f.ResponseToID = s
	}
	if flags&FlagAckID != 0 {
		s, ok := readShort()
		if !ok {
			return nil, ErrMalformed
		}
		f.AckID = s
	}
	if flags&FlagError != 0 {
		if pos+2 > len(data) {
			return nil, ErrMalformed
		}
		n := int(data[pos])<<8 | int(data[pos+1])
		pos += 2
		if pos+n > len(data) {
			return nil, ErrMalformed
		}
		f.Error = string(data[pos : pos+n])
		pos += n
	}

	// Always a subslice of data (possibly zero-length), never nil: binary is
	// always "present" as the remainder of the frame, per the wire spec, so
	// a zero-length remainder must decode the same as any other remainder —
	// not as a Go-level nil that a strict comparison could tell apart from
	// hex.DecodeString("")'s []byte{}.
	f.Binary = data[pos:]
	return f, nil
}
