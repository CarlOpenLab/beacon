//go:build !windows

package session

import (
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// The last reference to the PTY descriptor is the read that returns EOF. pump is
// still blocked in that read when the close marks the descriptor gone, and it is
// the read coming back afterwards — the shell's last output — that drops the
// reference and clears the descriptor out from under everyone else. A client
// resizing the pane in that same second asks the kernel for the window size on
// that very descriptor, so the two must not meet.
//
// A real PTY rather than a stand-in: the race is between the runtime's own
// bookkeeping and a kernel call, and a stand-in would have to reimplement the
// runtime to show it.
func TestResizeRacingTheReadThatFinishesDoesNotRaceOnTheDescriptor(t *testing.T) {
	// One pass is a coin flip; a hundred passes is a certainty.
	for i := 0; i < 100; i++ {
		master, slave, err := pty.Open()
		if err != nil {
			t.Skipf("no pty: %v", err)
		}
		s := newBareSession(&unixPTY{File: master})
		s.alive = true

		pumping := make(chan struct{})
		go func() {
			close(pumping)
			s.pump()
		}()
		<-pumping
		// Let pump reach its blocking Read.
		time.Sleep(time.Millisecond)

		// Mark the descriptor closed. It stays standing for now, because pump's
		// Read still holds the reference that would destroy it.
		_ = s.closePTY()

		// Now drop that last reference and resize at the same time: closing the
		// slave end returns EOF to pump, which is what clears the descriptor the
		// resize's ioctl is reading.
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_ = s.Resize(100, 30)
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = slave.Close()
		}()
		close(start)
		wg.Wait()
		_ = master.Close()
	}
}

// The same descriptor, the same race, from the other caller: the foreground poll
// asks the kernel which process group owns the terminal once every half second,
// for as long as the shell lives — which is every second a resizing client and a
// closing session share with it.
//
// This one goes straight to the PTY rather than through the session, because
// Session.Foreground holds the lock that keeps the two owners of the close
// apart, and holding it here would serialise the pollers away from each other
// until none of them overlapped the read that ends. The descriptor is the same
// one, and the call that reads it is the production one.
func TestForegroundRacingTheReadThatFinishesDoesNotRaceOnTheDescriptor(t *testing.T) {
	// One pass is a coin flip; a hundred passes is a certainty.
	for i := 0; i < 100; i++ {
		master, slave, err := pty.Open()
		if err != nil {
			t.Skipf("no pty: %v", err)
		}
		p := &unixPTY{File: master}
		s := newBareSession(p)
		s.alive = true

		pumping := make(chan struct{})
		go func() {
			close(pumping)
			s.pump()
		}()
		<-pumping
		// Let pump reach its blocking Read.
		time.Sleep(time.Millisecond)

		// Mark the descriptor closed. It stays standing for now, because pump's
		// Read still holds the reference that would destroy it.
		_ = s.closePTY()

		// Now drop that last reference and poll the foreground at the same time:
		// closing the slave end returns EOF to pump, which is what clears the
		// descriptor the poll's ioctl is reading.
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _ = p.Foreground()
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = slave.Close()
		}()
		close(start)
		wg.Wait()
		_ = master.Close()
	}
}
