package protocol

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"sync"
)

// Compression of the Data field, negotiated by CapDeflate in the hello.
//
// zlib rather than raw DEFLATE on purpose: the browser client answers frames
// with CompressionStream, and Web's format for "deflate" is the zlib wrapper.
// A Go hub and a Go window would be happy with either; the third client is not,
// and it is the one that runs on a phone.

var (
	zlibWriters = sync.Pool{New: func() any {
		w, _ := zlib.NewWriterLevel(io.Discard, zlib.BestSpeed)
		return w
	}}
)

// HasCap reports whether caps names one capability.
func HasCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// WorthCompressing reports whether b is large enough that a zlib wrapper is
// smaller than the header it costs.
func WorthCompressing(b []byte) bool { return len(b) >= minCompress }

// Deflate compresses b with zlib at BestSpeed. Terminal output is text with
// long repeats, so speed matters more than ratio; a shell that echoes a build
// log must not wait on a compressor to catch up.
func Deflate(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(b)/2 + 32)

	w := zlibWriters.Get().(*zlib.Writer)
	w.Reset(&buf)
	if _, err := w.Write(b); err != nil {
		zlibWriters.Put(w)
		return nil, err
	}
	if err := w.Close(); err != nil {
		zlibWriters.Put(w)
		return nil, err
	}
	zlibWriters.Put(w)
	return buf.Bytes(), nil
}

// Inflate decompresses b, refusing to produce more than limit bytes. The limit
// is the point: a peer that can compress chooses the ratio, and an unbounded
// reader would let one frame allocate without bound.
func Inflate(b []byte, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = MaxDataInflate
	}
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var out bytes.Buffer
	out.Grow(min(len(b)*4, limit))
	n, err := io.Copy(&out, io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if n > int64(limit) {
		return nil, errors.New("protocol: compressed frame expands past its limit")
	}
	return out.Bytes(), nil
}
