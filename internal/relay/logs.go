package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogStore keeps the tail of every machine's diagnostic log. A machine pushes
// its own lines over the relay's agent link, and the store persists the latest
// report so a relay restart does not erase the evidence an operator needs.
type LogStore struct {
	mu       sync.Mutex
	byID     map[string]LogSummary
	memory   map[string]*LogEntry
	maxLines int
	maxBytes int
	root     string
}

// LogEntry is one machine's latest report, with the metadata the panel shows
// beside it. Lines are oldest first, exactly as the machine wrote them.
type LogEntry struct {
	ID         string    `json:"id"`
	Version    string    `json:"version,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	Arch       string    `json:"arch,omitempty"`
	Level      string    `json:"level,omitempty"`
	Lines      []string  `json:"lines"`
	Bytes      int       `json:"bytes"`
	ReceivedAt time.Time `json:"receivedAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// LogSummary is what the list endpoint sends: everything except the lines.
type LogSummary struct {
	ID         string    `json:"id"`
	Version    string    `json:"version,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	Arch       string    `json:"arch,omitempty"`
	Level      string    `json:"level,omitempty"`
	Lines      int       `json:"lines"`
	Bytes      int       `json:"bytes"`
	ReceivedAt time.Time `json:"receivedAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// LogReport is the JSON a machine posts. It is a small envelope, not a raw log
// file: the relay needs to know which level was in force so a panel can say
// "this is only Info" rather than wonder why debug lines are missing.
type LogReport struct {
	Version   string   `json:"version,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Arch      string   `json:"arch,omitempty"`
	Level     string   `json:"level,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
	Lines     []string `json:"lines"`
}

const (
	defaultMaxLogLines = 4000
	defaultMaxLogBytes = 2 << 20
	maxLogLineBytes    = 8 << 10
)

// NewLogStore builds the in-memory store used by tests and embedders. The
// relay command uses NewPersistentLogStore so reports survive restarts.
func NewLogStore() *LogStore {
	return newLogStore("")
}

// NewPersistentLogStore loads machine reports from dir. Each report is kept in
// its own JSON file, which makes one corrupt machine unable to hide the others.
func NewPersistentLogStore(dir string) (*LogStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("relay: log directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	s := newLogStore(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var e LogEntry
		if err := json.Unmarshal(b, &e); err != nil || e.ID == "" || entry.Name() != logFileName(e.ID) {
			// A partial/corrupt report is ignored; the next upload repairs it.
			continue
		}
		e = s.normaliseEntry(e)
		s.byID[e.ID] = summaryOf(e)
	}
	return s, nil
}

func newLogStore(root string) *LogStore {
	return &LogStore{
		byID:     map[string]LogSummary{},
		memory:   map[string]*LogEntry{},
		maxLines: defaultMaxLogLines,
		maxBytes: defaultMaxLogBytes,
		root:     root,
	}
}

// Put replaces a machine's report with the newest one. Persistence happens
// before the in-memory index is swapped, so a failed disk write never makes a
// report appear saved when it is not.
func (s *LogStore) Put(id string, report LogReport) (LogEntry, error) {
	now := time.Now().UTC()
	updated := now
	if report.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, report.UpdatedAt); err == nil {
			updated = t
		}
	}
	e := s.normaliseEntry(LogEntry{
		ID: id, Version: report.Version, Platform: report.Platform, Arch: report.Arch,
		Level: report.Level, Lines: append([]string(nil), report.Lines...),
		ReceivedAt: now, UpdatedAt: updated,
	})
	s.mu.Lock()
	if err := s.persistLocked(e); err != nil {
		s.mu.Unlock()
		return e, err
	}
	s.byID[id] = summaryOf(e)
	if s.root == "" {
		s.memory[id] = &e
	}
	s.mu.Unlock()
	return e, nil
}

func summaryOf(e LogEntry) LogSummary {
	return LogSummary{
		ID: e.ID, Version: e.Version, Platform: e.Platform, Arch: e.Arch,
		Level: e.Level, Lines: len(e.Lines), Bytes: e.Bytes,
		ReceivedAt: e.ReceivedAt, UpdatedAt: e.UpdatedAt,
	}
}

func (s *LogStore) normaliseEntry(e LogEntry) LogEntry {
	lines := make([]string, 0, len(e.Lines))
	bytes := 0
	for _, line := range e.Lines {
		if len(line) > maxLogLineBytes {
			line = line[:maxLogLineBytes]
		}
		lines = append(lines, line)
		bytes += len(line) + 1
		for len(lines) > s.maxLines || bytes > s.maxBytes {
			bytes -= len(lines[0]) + 1
			lines = lines[1:]
		}
	}
	e.Lines, e.Bytes = lines, bytes
	return e
}

func (s *LogStore) persistLocked(e LogEntry) error {
	if s.root == "" {
		return nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	path := filepath.Join(s.root, logFileName(e.ID))
	tmp, err := os.CreateTemp(s.root, ".log-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func logFileName(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "machine-" + hex.EncodeToString(sum[:]) + ".json"
}

// Get returns one machine's report, lines included.
func (s *LogStore) Get(id string) (LogEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.memory[id]
	if ok {
		cp := *e
		cp.Lines = append([]string(nil), e.Lines...)
		return cp, true
	}
	_, ok = s.byID[id]
	if !ok {
		return LogEntry{}, false
	}
	if s.root == "" {
		return LogEntry{}, false
	}
	b, err := os.ReadFile(filepath.Join(s.root, logFileName(id)))
	if err != nil {
		return LogEntry{}, false
	}
	var out LogEntry
	if err := json.Unmarshal(b, &out); err != nil || out.ID != id {
		return LogEntry{}, false
	}
	out = s.normaliseEntry(out)
	return out, true
}

func (s *LogStore) List() []LogSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogSummary, 0, len(s.byID))
	for _, e := range s.byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Clear forgets one machine's report and removes its persisted copy.
func (s *LogStore) Clear(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	if s.root != "" {
		if err := os.Remove(filepath.Join(s.root, logFileName(id))); err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	delete(s.byID, id)
	delete(s.memory, id)
	return ok, nil
}

func (e LogEntry) Text() string { return strings.Join(e.Lines, "\n") }
