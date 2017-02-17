package uartgw

import "io"

// escapingWriter escapes 0xfd for the UARTGW
type escapingWriter struct {
	w io.Writer
}

func (ew *escapingWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// Twice as long: in the worst case, every byte needs to be escaped.
	escaped := make([]byte, 0, len(p)*2)
	for _, b := range p {
		// 0xfd (frame delimiter) must be escaped within a frame.
		// 0xfc introduces an escaped byte, so bytes which happen to
		// be 0xfc need to be escaped as well.
		if b == 0xfd || b == 0xfc {
			escaped = append(escaped, 0xfc, b&0x7f)
		} else {
			escaped = append(escaped, b)
		}
	}
	n, err = ew.w.Write(escaped)
	if err != nil {
		return n, err
	}
	return len(p), nil
}

type unescapingReader struct {
	r io.Reader
}

func (uer *unescapingReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	raw := make([]byte, len(p))
	n, err = uer.r.Read(raw)
	if err != nil {
		return n, err
	}
	var escapeByte bool
	idx := 0
	for _, b := range raw[:n] {
		if b == 0xfc {
			escapeByte = true
			continue
		}
		if escapeByte {
			b |= 0x80
			escapeByte = false
		}
		p[idx] = b
		idx++
	}

	// We cannot end on an escaped byte because the escape state would
	// not be carried over into the next read call. Force a read.
	if escapeByte {
		last := make([]byte, 1)
		n, err = uer.r.Read(last)
		if err != nil {
			return idx, err
		}
		p[idx] = last[0] | 0x80
		idx++
	}
	return idx, nil
}
