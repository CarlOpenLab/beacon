//go:build !windows

package hub_test

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// dialRaw is dialTest for a client that announces capabilities.
func dialRaw(t *testing.T, ts *httptest.Server, caps []string) *conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/"
	wsc, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = wsc.Close(websocket.StatusNormalClosure, "test done") })

	c := &conn{t: t, conn: wsc}
	c.send(protocol.Msg{T: protocol.CTHello, V: protocol.Version, Caps: caps})
	hello := c.waitFor("hello", func(m protocol.Msg) bool { return m.T == protocol.STHello })
	if !protocol.HasCap(hello.Caps, protocol.CapFile) {
		t.Fatalf("hub did not answer file.put: caps=%v", hello.Caps)
	}
	return c
}

func TestPasteUploadRoundTrip(t *testing.T) {
	c := dialRaw(t, newTestHub(t), []string{protocol.CapDeflate, protocol.CapFile})
	c.create("s1")

	payload := []byte("a pasted screenshot, in bytes")
	req := "p1"
	c.send(protocol.Msg{
		T: protocol.CTFilePut, ReqID: req, SessionID: "s1",
		Name: "shot.png", Size: int64(len(payload)),
	})
	c.send(protocol.Msg{T: protocol.CTFileChunk, ReqID: req, SessionID: "s1", Data: payload[:10]})
	c.send(protocol.Msg{T: protocol.CTFileChunk, ReqID: req, SessionID: "s1", Data: payload[10:]})
	c.send(protocol.Msg{T: protocol.CTFileDone, ReqID: req, SessionID: "s1"})

	saved := c.waitFor("file.saved", func(m protocol.Msg) bool {
		return m.T == protocol.STFileSaved && m.ReqID == req
	})
	if saved.Code != "" {
		t.Fatalf("upload refused: %s (%s)", saved.Message, saved.Code)
	}
	if !strings.HasSuffix(saved.Path, "-shot.png") {
		t.Fatalf("path %q does not keep the name", saved.Path)
	}
	if !strings.Contains(saved.Path, "beacon-paste") {
		t.Fatalf("path %q is not in the paste directory", saved.Path)
	}
	t.Cleanup(func() { _ = os.Remove(saved.Path) })
	got, err := os.ReadFile(saved.Path)
	if err != nil {
		t.Fatalf("read upload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("uploaded bytes = %q, want %q", got, payload)
	}
}

func TestPasteUploadRefusedForViewer(t *testing.T) {
	ts := newTestHub(t)
	owner := dialRaw(t, ts, nil)
	owner.create("s1")

	viewer := dialRaw(t, ts, []string{protocol.CapFile})
	viewer.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	viewer.waitFor("role", func(m protocol.Msg) bool { return m.T == protocol.STRole && m.Role == protocol.RoleViewer })

	viewer.send(protocol.Msg{
		T: protocol.CTFilePut, ReqID: "p1", SessionID: "s1", Name: "x.png", Size: 3,
	})
	fail := viewer.waitFor("refusal", func(m protocol.Msg) bool {
		return m.T == protocol.STError && m.Code == protocol.ErrNotController
	})
	if fail.Message == "" {
		t.Fatal("refusal explained nothing")
	}
}

func TestPasteUploadRejectsLyingSize(t *testing.T) {
	c := dialRaw(t, newTestHub(t), []string{protocol.CapFile})
	c.create("s1")

	c.send(protocol.Msg{
		T: protocol.CTFilePut, ReqID: "p1", SessionID: "s1", Name: "big.bin", Size: 4,
	})
	c.send(protocol.Msg{T: protocol.CTFileChunk, ReqID: "p1", SessionID: "s1", Data: []byte("12345")})
	saved := c.waitFor("refusal", func(m protocol.Msg) bool {
		return m.T == protocol.STFileSaved && m.ReqID == "p1"
	})
	if saved.Code == "" {
		t.Fatalf("a chunk past the declared size was accepted (path %q)", saved.Path)
	}
}

func TestCompressedOutputRoundTrip(t *testing.T) {
	c := dialRaw(t, newTestHub(t), []string{protocol.CapDeflate, protocol.CapFile})
	c.create("s1")
	c.send(protocol.Msg{T: protocol.CTAttach, SessionID: "s1", Cursor: 0})
	c.waitFor("attached", func(m protocol.Msg) bool { return m.T == protocol.STAttached })

	// A line long enough that the hub compresses its frame; short echoes stay
	// plain on purpose.
	c.send(protocol.Msg{T: protocol.CTInput, SessionID: "s1", Data: []byte("printf '%0500d\\n' 7\n")})

	deadline := time.Now().Add(20 * time.Second)
	var got strings.Builder
	for time.Now().Before(deadline) {
		m := c.recv()
		if m.T != protocol.STOutput {
			continue
		}
		if !m.Z {
			got.Write(m.Data)
			continue
		}
		out, err := protocol.Inflate(m.Data, protocol.MaxDataInflate)
		if err != nil {
			t.Fatalf("inflate: %v", err)
		}
		got.Write(out)
		if strings.Contains(got.String(), strings.Repeat("0", 400)) {
			return
		}
	}
	t.Fatalf("no compressed output arrived; got %d bytes", got.Len())
}
