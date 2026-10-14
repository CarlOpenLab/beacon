package session

import "testing"

func TestRingReadFrom(t *testing.T) {
	r := NewRing(1024)
	r.Write([]byte("hello"))

	data, from, trunc := r.ReadFrom(0)
	if string(data) != "hello" || from != 0 || trunc {
		t.Fatalf("ReadFrom(0) = %q from=%d trunc=%v", data, from, trunc)
	}
	if r.End() != 5 {
		t.Fatalf("End() = %d, want 5", r.End())
	}

	data, from, trunc = r.ReadFrom(3)
	if string(data) != "lo" || from != 3 || trunc {
		t.Fatalf("ReadFrom(3) = %q from=%d trunc=%v", data, from, trunc)
	}
}

func TestRingDropsOldest(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("abcdefgh"))

	if r.Start() != 4 || r.End() != 8 {
		t.Fatalf("Start=%d End=%d, want 4 and 8", r.Start(), r.End())
	}
	data, from, trunc := r.ReadFrom(0)
	if !trunc || from != 4 || string(data) != "efgh" {
		t.Fatalf("ReadFrom(0) = %q from=%d trunc=%v, want \"efgh\" from=4 trunc=true", data, from, trunc)
	}
}

func TestRingOffsetBeyondEnd(t *testing.T) {
	r := NewRing(16)
	r.Write([]byte("abc"))

	data, from, trunc := r.ReadFrom(99)
	if len(data) != 0 || from != 99 || trunc {
		t.Fatalf("ReadFrom(99) = %q from=%d trunc=%v, want empty at 99", data, from, trunc)
	}
}

func TestRingAccumulates(t *testing.T) {
	r := NewRing(64)
	r.Write([]byte("one"))
	r.Write([]byte("two"))

	data, _, _ := r.ReadFrom(0)
	if string(data) != "onetwo" {
		t.Fatalf("got %q, want \"onetwo\"", data)
	}
	data, from, _ := r.ReadFrom(3)
	if string(data) != "two" || from != 3 {
		t.Fatalf("got %q from=%d, want \"two\" from=3", data, from)
	}
}
