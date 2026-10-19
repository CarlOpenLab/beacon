package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/tunnel"
)

// A full out queue must slow a viewer's writer down, not answer it with an
// error the hub reads as a dead client. The error path is what killed a viewer
// mid-replay on a busy session: the hub dropped the client, the replay frame
// never reached the tab, and the pane stayed blank with a live shell behind it.
func TestSendCtxWaitsForRoomInsteadOfDropping(t *testing.T) {
	s := &session{out: make(chan tunnel.Frame, 1), done: make(chan struct{})}
	if err := s.sendCtx(context.Background(), tunnel.Frame{T: tunnel.TData}); err != nil {
		t.Fatalf("send into an empty queue: %v", err)
	}

	// The queue is full and nobody is draining: the send waits for the ctx
	// rather than failing at once.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := s.sendCtx(ctx, tunnel.Frame{T: tunnel.TData})
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("send returned after %s: it dropped the frame instead of waiting for room", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("send on a full queue: %v, want the ctx's deadline", err)
	}

	// A link that is gone answers at once, whichever way the queue stands.
	close(s.done)
	if err := s.sendCtx(context.Background(), tunnel.Frame{T: tunnel.TData}); err == nil {
		t.Error("send on a closed link succeeded")
	}
}

// Room appearing is what the wait is for: the frame goes out as soon as the
// writeLoop drains one, without the hub's writer ever seeing an error.
func TestSendCtxProceedsWhenRoomAppears(t *testing.T) {
	s := &session{out: make(chan tunnel.Frame, 1), done: make(chan struct{})}
	if err := s.sendCtx(context.Background(), tunnel.Frame{T: tunnel.TData, Stream: 7}); err != nil {
		t.Fatalf("fill the queue: %v", err)
	}
	got := make(chan error, 1)
	go func() {
		got <- s.sendCtx(context.Background(), tunnel.Frame{T: tunnel.TData, Stream: 8})
	}()
	time.Sleep(20 * time.Millisecond)
	<-s.out // the writeLoop taking one frame
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("send after room appeared: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the waiting send never completed")
	}
	if f := <-s.out; f.Stream != 8 {
		t.Fatalf("frame sent = %d, want 8", f.Stream)
	}
}
