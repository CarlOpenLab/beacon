package main

// Pasting an image, or a file, into a shell.
//
// The clipboard belongs to the machine the person is sitting at; the shell may
// be running somewhere else. A pasted screenshot is therefore not text to send
// down the pipe: it has to become a file on the machine that owns the PTY, and
// what the program is given is that file's path. The upload is the hub's
// file.put (see internal/hub/upload.go), which writes into a private temp
// directory and answers with an absolute path.
//
// The terminal hands every paste to OnPaste before the program sees it, which
// is the hook this file fills: a paste that carries only text is left to the
// terminal, and one that carries an image or files is taken here.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"

	"github.com/CarlOpenLab/beacon/internal/diag"
	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/locales"
)

const (
	// maxPasteFiles bounds one paste. A clipboard with a directory in it would
	// otherwise turn a keystroke into a long upload.
	maxPasteFiles = 8
	// pasteTimeout bounds one file's upload, from the first frame to the path
	// coming back. A relay stream that has gone quiet never answers.
	pasteTimeout = 45 * time.Second
)

// pasteItem is one file a paste will carry.
type pasteItem struct {
	name string
	data []byte
}

// pasteInto is the terminal's OnPaste. It runs on the main thread and must
// decide at once, so it reads only the clipboard's format list here and does
// the reading and the uploading on a goroutine.
//
// It reports whether it took the paste: true for an image or a file, false for
// text, which the terminal pastes itself exactly as it always did.
func (w *window) pasteInto(tb *tab, d transfer.Data) bool {
	if tb == nil || tb.sess == nil || tb.sess.link == nil {
		return false
	}
	formats := d.Formats()
	hasFile := false
	hasImage := false
	for _, f := range formats {
		switch f {
		case transfer.FileList, transfer.URIList:
			hasFile = true
		default:
			if strings.HasPrefix(string(f), "image/") {
				hasImage = true
			}
		}
	}
	// One line per paste, whatever it turns out to be: which machine, which
	// session, what the clipboard offers, and what was recognised in it. A
	// paste that does nothing has to leave a trace that says where it stopped.
	diag.Infof("paste", "clipboard machine=%s session=%s formats=%v files=%v image=%v",
		tb.machine, tb.id, formats, hasFile, hasImage)
	if !hasFile && !hasImage {
		diag.Infof("paste", "not a file or image: left to the terminal")
		return false
	}

	if tb.known() && !tb.held() {
		diag.Infof("paste", "refused: read-only machine=%s session=%s", tb.machine, tb.id)
		w.update(func() { w.setNotice(locales.PasteReadOnly()) })
		return true
	}
	if !tb.sess.link.capable(protocol.CapFile) {
		diag.Infof("paste", "refused: the machine does not take files machine=%s caps=%v",
			tb.machine, tb.sess.link.capsList())
		w.update(func() { w.setNotice(locales.PasteUnsupported()) })
		return true
	}

	snapshot := d.Snapshot()
	machine := tb.machine
	go w.uploadPaste(tb, snapshot, hasFile, hasImage, machine)
	return true
}

// uploadPaste turns the clipboard into items and sends them one at a time, then
// pastes the paths into the shell.
func (w *window) uploadPaste(tb *tab, d transfer.Data, hasFile, hasImage bool, machine string) {
	items, reason := pasteItems(d, hasFile, hasImage)
	diag.Infof("paste", "read machine=%s session=%s items=%d reason=%q", machine, tb.id, len(items), reason)
	if reason != "" {
		w.update(func() { w.setNotice(reason) })
		return
	}
	if len(items) > maxPasteFiles {
		items = items[:maxPasteFiles]
	}

	link := tb.sess.link
	paths := make([]string, 0, len(items))
	for _, item := range items {
		path, code, message, ok := link.putFile(tb.id, item.name, item.data)
		if !ok {
			line := message
			if line == "" {
				line = code
			}
			diag.Warnf("paste", "upload failed machine=%s session=%s code=%s message=%q", machine, tb.id, code, message)
			w.update(func() { w.setNotice(locales.PasteFailed(line)) })
			return
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}

	quoted := make([]string, 0, len(paths))
	platform := runtime.GOOS
	if m, ok := w.machineByID(machine); ok && m.Host.Platform != "" {
		platform = m.Host.Platform
	}
	for _, path := range paths {
		quoted = append(quoted, shellQuote(path, platform))
	}
	text := strings.Join(quoted, " ") + " "
	diag.Infof("paste", "uploaded machine=%s session=%s paths=%v text=%q", machine, tb.id, paths, text)
	w.update(func() {
		if tb.term == nil {
			return
		}
		tb.term.Paste(text)
	})
}

// pasteItems reads the clipboard's files and image into the bytes to upload.
// It returns a reason instead when the clipboard has nothing it can send, or
// something too large.
func pasteItems(d transfer.Data, hasFile, hasImage bool) ([]pasteItem, string) {
	var items []pasteItem
	if hasFile {
		files, err := d.Files()
		if err != nil {
			return nil, locales.PasteFailed(err.Error())
		}
		for _, path := range files {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			if info.Size() > protocol.MaxUploadBytes {
				return nil, locales.PasteTooLarge(filepath.Base(path))
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, locales.PasteFailed(err.Error())
			}
			items = append(items, pasteItem{name: filepath.Base(path), data: data})
		}
	}
	if len(items) == 0 && hasImage {
		data, err := d.Read(transfer.PNG)
		if err != nil || len(data) == 0 {
			if err == nil {
				return nil, locales.PasteFailed("empty image")
			}
			return nil, locales.PasteFailed(err.Error())
		}
		if len(data) > protocol.MaxUploadBytes {
			return nil, locales.PasteTooLarge("screenshot.png")
		}
		items = append(items, pasteItem{name: "screenshot.png", data: data})
	}
	if len(items) == 0 {
		return nil, locales.PasteFailed("nothing to send")
	}
	return items, ""
}

// putFile uploads one file to the machine behind this link and waits for the
// path it landed at. The frames carry a request id, so an answer can be matched
// to the paste that started it even when two tabs paste at once.
func (l *link) putFile(session, name string, data []byte) (path, code, message string, ok bool) {
	if len(data) == 0 || len(data) > protocol.MaxUploadBytes {
		return "", protocol.ErrBadRequest, "the file is empty or too large", false
	}
	if !l.online() {
		return "", protocol.ErrBadRequest, "the machine is not reachable", false
	}

	l.putMu.Lock()
	l.nextPut++
	reqID := fmt.Sprintf("p%d", l.nextPut)
	answers := make(chan protocol.Msg, 1)
	l.puts[reqID] = func(m protocol.Msg) { answers <- m }
	l.putMu.Unlock()
	defer func() {
		l.putMu.Lock()
		delete(l.puts, reqID)
		l.putMu.Unlock()
	}()

	l.sendMsg(protocol.Msg{
		T:         protocol.CTFilePut,
		ReqID:     reqID,
		SessionID: session,
		Name:      name,
		Size:      int64(len(data)),
		Epoch:     l.epochOf(session),
	})
	for off := 0; off < len(data); off += protocol.MaxUploadChunk {
		end := off + protocol.MaxUploadChunk
		if end > len(data) {
			end = len(data)
		}
		l.sendMsg(protocol.Msg{
			T:         protocol.CTFileChunk,
			ReqID:     reqID,
			SessionID: session,
			Data:      data[off:end],
		})
	}
	l.sendMsg(protocol.Msg{T: protocol.CTFileDone, ReqID: reqID, SessionID: session})

	select {
	case m := <-answers:
		if m.Code != "" {
			return "", m.Code, m.Message, false
		}
		if m.Path == "" {
			return "", protocol.ErrBadRequest, "the machine sent no path", false
		}
		return m.Path, "", "", true
	case <-time.After(pasteTimeout):
		return "", protocol.ErrBadRequest, "the upload timed out", false
	}
}

// deliverFile hands an upload's answer to the paste waiting for it.
func (l *link) deliverFile(m protocol.Msg) {
	l.putMu.Lock()
	fn := l.puts[m.ReqID]
	l.putMu.Unlock()
	if fn == nil {
		return
	}
	fn(m)
}

// failPuts ends every upload waiting on a link that has gone away, so a paste
// reports the lost machine instead of waiting out its timeout.
func (l *link) failPuts(reason string) {
	l.putMu.Lock()
	waiters := make([]func(protocol.Msg), 0, len(l.puts))
	for id, fn := range l.puts {
		waiters = append(waiters, fn)
		delete(l.puts, id)
	}
	l.putMu.Unlock()
	for _, fn := range waiters {
		fn(protocol.Msg{Code: protocol.ErrBadRequest, Message: reason})
	}
}

// shellQuote makes a path one word for the shell on the far machine: single
// quotes for POSIX shells and PowerShell alike, with the character that would
// close them doubled in each dialect's own way.
func shellQuote(path, platform string) string {
	if platform == "windows" {
		return "'" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
