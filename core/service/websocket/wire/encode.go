package wire

// Encode serialises f. It returns ErrFieldTooLong if any field exceeds its
// on-wire length limit; silent truncation would corrupt the stream.
// Empty optional fields (ID, ResponseToID, AckID, Error) are omitted from
// the wire entirely — their flag bit stays clear — so empty and absent are
// indistinguishable on the wire; see the Frame doc comment.
func Encode(f *Frame) ([]byte, error) {
	if len(f.MsgType) > MaxShortField ||
		len(f.ID) > MaxShortField ||
		len(f.ResponseToID) > MaxShortField ||
		len(f.AckID) > MaxShortField {
		return nil, ErrFieldTooLong
	}
	if len(f.Error) > MaxErrorLen {
		return nil, ErrFieldTooLong
	}

	var flags byte
	if f.ID != "" {
		flags |= FlagID
	}
	if f.ResponseToID != "" {
		flags |= FlagResponseToID
	}
	if f.AckID != "" {
		flags |= FlagAckID
	}
	if f.Error != "" {
		flags |= FlagError
	}

	size := 2 + len(f.MsgType) + len(f.Binary)
	if f.MsgType == "" {
		size = 3 + len(f.Binary) // control code byte
	}
	if f.ID != "" {
		size += 1 + len(f.ID)
	}
	if f.ResponseToID != "" {
		size += 1 + len(f.ResponseToID)
	}
	if f.AckID != "" {
		size += 1 + len(f.AckID)
	}
	if f.Error != "" {
		size += 2 + len(f.Error)
	}

	buf := make([]byte, 0, size)
	buf = append(buf, Version|flags)

	if f.MsgType == "" {
		buf = append(buf, 0, f.ControlCode)
	} else {
		buf = append(buf, byte(len(f.MsgType)))
		buf = append(buf, f.MsgType...)
	}

	// Order matches flag bit order; decoders rely on it.
	if f.ID != "" {
		buf = append(buf, byte(len(f.ID)))
		buf = append(buf, f.ID...)
	}
	if f.ResponseToID != "" {
		buf = append(buf, byte(len(f.ResponseToID)))
		buf = append(buf, f.ResponseToID...)
	}
	if f.AckID != "" {
		buf = append(buf, byte(len(f.AckID)))
		buf = append(buf, f.AckID...)
	}
	if f.Error != "" {
		buf = append(buf, byte(len(f.Error)>>8), byte(len(f.Error)))
		buf = append(buf, f.Error...)
	}

	buf = append(buf, f.Binary...)
	return buf, nil
}
