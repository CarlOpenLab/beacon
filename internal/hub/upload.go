package hub

// Uploads: pasting an image or a file into a shell.
//
// The clipboard of the machine the person is sitting at is not the machine the
// shell runs on. A pasted screenshot therefore has to become a file on the
// machine that owns the PTY, and the only thing the program in that shell can
// be handed is a path. This file is that file's address on the far side: the
// client names it, streams it, and the hub writes it into a private directory
// it will read back from.
//
// It is deliberately small: three frames, one upload in flight per client, and
// a hard ceiling. The alternative — a file server behind the relay — would be a
// second protocol to keep secure, and this one is a paste.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/protocol"
)

// uploadStale is how long a started upload may go without finishing before the
// next one replaces it. A paste that was abandoned — a closed laptop, a stream
// that died mid-file — must not keep the client unable to paste again.
const uploadStale = 2 * time.Minute

// upload is one file arriving from a client.
type upload struct {
	reqID   string
	session string
	file    *os.File
	size    int64
	written int64
	started time.Time
}

// pasteDir is where uploads land: the temp directory of the user the daemon
// runs as, in a directory only that user can enter. It is the same kind of
// place an editor would write a scratch file, and the shell that reads it runs
// as the same user.
func pasteDir() string {
	return filepath.Join(os.TempDir(), "beacon-paste")
}

// handleFilePut starts an upload: it names the file, its size and the session
// it will be pasted into. The lease is checked here, not at each chunk: the
// right to write this file is the right to type into that shell.
func (h *Hub) handleFilePut(c *client, m protocol.Msg) {
	if !protocol.HasCap(c.caps(), protocol.CapFile) {
		c.failSaved(m, protocol.ErrBadRequest, "this client did not announce file.paste", "")
		return
	}
	if m.SessionID == "" {
		c.failSaved(m, protocol.ErrBadRequest, "file.put names no session", "")
		return
	}
	if !h.requireControl(c, m.SessionID, 0) {
		return
	}
	if m.Size <= 0 || m.Size > protocol.MaxUploadBytes {
		c.failSaved(m, protocol.ErrBadRequest, fmt.Sprintf(
			"a paste may be at most %d bytes", protocol.MaxUploadBytes), "")
		return
	}
	if m.ReqID == "" {
		m.ReqID = newUploadID()
	}

	c.abortUpload("replaced by a newer paste")
	dir := pasteDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.failSaved(m, protocol.ErrBadRequest, "cannot make a paste directory: "+err.Error(), "")
		return
	}
	path, f, err := createPasteFile(dir, m.Name)
	if err != nil {
		c.failSaved(m, protocol.ErrBadRequest, "cannot write the paste: "+err.Error(), "")
		return
	}
	c.mu.Lock()
	c.up = &upload{reqID: m.ReqID, session: m.SessionID, file: f, size: m.Size, started: time.Now()}
	c.upPath = path
	c.mu.Unlock()
	diag.Infof("hub", "paste started session=%s client=%s bytes=%d path=%s", m.SessionID, c.id, m.Size, path)
}

// handleFileChunk appends one piece. A chunk that would run past the size the
// client declared is refused and the file is removed: a peer that lies about a
// size does not get to write past it.
func (h *Hub) handleFileChunk(c *client, m protocol.Msg) {
	c.mu.Lock()
	u := c.up
	c.mu.Unlock()
	if u == nil || u.reqID != m.ReqID {
		c.failSaved(m, protocol.ErrBadRequest, "no upload named "+m.ReqID, "")
		return
	}
	if u.written+int64(len(m.Data)) > u.size {
		c.failSaved(m, protocol.ErrBadRequest, "the paste is larger than it declared", "")
		c.abortUpload("oversized")
		return
	}
	if _, err := u.file.Write(m.Data); err != nil {
		c.failSaved(m, protocol.ErrBadRequest, "cannot write the paste: "+err.Error(), "")
		c.abortUpload("write failed")
		return
	}
	u.written += int64(len(m.Data))
}

// handleFileDone finishes the upload and answers with the path the shell can
// read. An upload shorter than it declared is accepted at its actual length:
// a clipboard image can be read while it changes underneath the client, and a
// shorter file is still a whole file.
func (h *Hub) handleFileDone(c *client, m protocol.Msg) {
	c.mu.Lock()
	u := c.up
	path := c.upPath
	if u == nil || u.reqID != m.ReqID {
		c.mu.Unlock()
		c.failSaved(m, protocol.ErrBadRequest, "no upload named "+m.ReqID, "")
		return
	}
	c.up, c.upPath = nil, ""
	c.mu.Unlock()

	if err := u.file.Close(); err != nil {
		_ = os.Remove(path)
		c.failSaved(m, protocol.ErrBadRequest, "cannot finish the paste: "+err.Error(), "")
		return
	}
	if u.written == 0 {
		_ = os.Remove(path)
		c.failSaved(m, protocol.ErrBadRequest, "the paste was empty", "")
		return
	}
	diag.Infof("hub", "paste saved session=%s client=%s bytes=%d path=%s", u.session, c.id, u.written, path)
	c.send(protocol.Msg{
		T:         protocol.STFileSaved,
		ReqID:     u.reqID,
		SessionID: u.session,
		Path:      path,
		Size:      u.written,
	})
}

// abortUpload throws away a partial file, if there is one. The caller holds no
// lock: this takes the client's own.
func (c *client) abortUpload(reason string) {
	c.mu.Lock()
	u := c.up
	path := c.upPath
	c.up, c.upPath = nil, ""
	c.mu.Unlock()
	if u == nil {
		return
	}
	_ = u.file.Close()
	_ = os.Remove(path)
	diag.Infof("hub", "paste abandoned client=%s reason=%s path=%s", c.id, reason, path)
}

// failSaved answers an upload frame with the failure the client will show. It
// carries the request so the client can match it to the paste that started it.
func (c *client) failSaved(m protocol.Msg, code, message, path string) {
	c.send(protocol.Msg{
		T:         protocol.STFileSaved,
		ReqID:     m.ReqID,
		SessionID: m.SessionID,
		Path:      path,
		Code:      code,
		Message:   message,
	})
}

// createPasteFile makes the file the upload writes into: a name derived from
// the client's, never the client's path, in a directory the daemon owns. The
// timestamp keeps a screenshot pasted twice from replacing the first one, and
// the random suffix keeps two clients apart when their clocks agree.
func createPasteFile(dir, name string) (string, *os.File, error) {
	base := pasteName(name)
	for i := 0; i < 8; i++ {
		tag := make([]byte, 3)
		if _, err := rand.Read(tag); err != nil {
			return "", nil, err
		}
		path := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+hex.EncodeToString(tag)+"-"+base)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return path, f, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("cannot find a free name")
}

// pasteName is the client's filename, reduced to something that cannot escape
// the paste directory or surprise a shell: no separators, no controls, no
// leading dots, a bounded length, and an extension kept only if it looks like
// one.
func pasteName(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimLeft(base, ".")
	var b strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "paste"
	}
	return out
}

// newUploadID is a request id for a client that sent none.
func newUploadID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("u%d", time.Now().UnixNano())
	}
	return "u" + hex.EncodeToString(b)
}
