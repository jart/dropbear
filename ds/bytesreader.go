package ds

import "io"

// NewBytesReader returns io.Reader for byte slice.
func NewBytesReader(b []byte) io.Reader {
	return &bytesReader{b: b}
}

type bytesReader struct {
	b []byte
	o int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.o >= len(r.b) {
		return 0, io.EOF
	}
	n = copy(p, r.b[r.o:])
	r.o += n
	return n, nil
}
