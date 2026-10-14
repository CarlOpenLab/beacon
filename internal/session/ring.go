package session

import "sync"

// Ring keeps the recent output of one PTY in a byte-offset address space.
//
// Offsets are absolute and monotonic: they never reset, so a client that
// reconnects can say "give me everything after N" and receive only the gap
// instead of a replay. Once the buffer exceeds its limit the oldest bytes are
// dropped from the front and ReadFrom reports that as truncated.
type Ring struct {
	mu    sync.Mutex
	buf   []byte
	start int64 // absolute offset of buf[0]
	total int64 // absolute offset one past the newest byte
	limit int
}

func NewRing(limit int) *Ring {
	if limit <= 0 {
		limit = 4096
	}
	return &Ring{limit: limit}
}

// Write appends p, evicting the oldest bytes if the buffer overflows.
func (r *Ring) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf = append(r.buf, p...)
	r.total += int64(len(p))
	if len(r.buf) > r.limit {
		drop := len(r.buf) - r.limit
		n := copy(r.buf, r.buf[drop:])
		r.buf = r.buf[:n]
		r.start += int64(drop)
	}
}

// ReadFrom returns the bytes from offset up to the newest byte.
//
// from is where the returned slice actually begins; it differs from offset
// exactly when truncated is true, which tells the caller that history fell
// out of the ring and the client must resynchronise rather than append.
// An offset beyond the newest byte yields an empty slice, not an error.
func (r *Ring) ReadFrom(offset int64) (data []byte, from int64, truncated bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	from = offset
	if from < r.start {
		from = r.start
		truncated = true
	}
	if from >= r.total {
		return nil, from, truncated
	}
	i := int(from - r.start)
	out := make([]byte, len(r.buf)-i)
	copy(out, r.buf[i:])
	return out, from, truncated
}

// End is the offset one past the newest byte, i.e. the cursor a client holds
// once it has seen everything.
func (r *Ring) End() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

// Start is the oldest offset still held.
func (r *Ring) Start() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.start
}
