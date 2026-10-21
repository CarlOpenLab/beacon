package relay

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// LogStore keeps the tail of every machine's diagnostic log. A machine pushes
// its own lines over the relay's agent link (an HTTP POST beside the tunnel, so
// the relay still never parses an inner frame), and the admin panel reads them
// from here.
//
// The store is deliberately a ring rather than a file: a relay is a router, not
// a logging service. It holds enough recent history for someone to look at a
// bug while it is still warm, and never grows without bound.
type LogStore struct {
	mu       sync.Mutex
	byID     map[string]*LogEntry
	maxLines int
	maxBytes int
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
	// UpdatedAt is when the machine produced its newest line, from the report.
	// It can lag ReceivedAt when a machine has been quiet.
	UpdatedAt time.Time `json:"updatedAt"`
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
	// defaultMaxLogBytes bounds one machine's stored report. A debug log line
	// is a few hundred bytes; 2 MiB holds many thousands of them.
	defaultMaxLogBytes = 2 << 20
	maxLogLineBytes    = 8 << 10
)

// NewLogStore builds the store the relay uses.
func NewLogStore() *LogStore {
	return &LogStore{
		byID:     map[string]*LogEntry{},
		maxLines: defaultMaxLogLines,
		maxBytes: defaultMaxLogBytes,
	}
}

// Put replaces a machine's report with the newest one. Reports are full tails,
// not deltas, so replacing is what makes the panel show one machine's story
// rather than an interleaving of every upload.
func (s *LogStore) Put(id string, report LogReport) LogEntry {
	now := time.Now().UTC()
	lines := make([]string, 0, len(report.Lines))
	bytes := 0
	for _, line := range report.Lines {
		if len(line) > maxLogLineBytes {
			line = line[:maxLogLineBytes]
		}
		// The cap is kept from the end: the newest lines are the ones that
		// explain the thing that just went wrong.
		bytes += len(line) + 1
		lines = append(lines, line)
		if len(lines) > s.maxLines || bytes > s.maxBytes {
			drop := len(lines) - s.maxLines
			if drop < 1 {
				drop = 1
			}
			for i := 0; i < drop; i++ {
				bytes -= len(lines[i]) + 1
			}
			lines = lines[drop:]
		}
	}
	updated := now
	if report.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, report.UpdatedAt); err == nil {
			updated = t
		}
	}
	entry := &LogEntry{
		ID:         id,
		Version:    report.Version,
		Platform:   report.Platform,
		Arch:       report.Arch,
		Level:      report.Level,
		Lines:      lines,
		Bytes:      bytes,
		ReceivedAt: now,
		UpdatedAt:  updated,
	}
	s.mu.Lock()
	s.byID[id] = entry
	s.mu.Unlock()
	return *entry
}

// Get returns one machine's report, lines included.
func (s *LogStore) Get(id string) (LogEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return LogEntry{}, false
	}
	cp := *e
	cp.Lines = append([]string(nil), e.Lines...)
	return cp, true
}

// List returns one summary per machine, sorted by id so the panel's rows do not
// reshuffle between refreshes.
func (s *LogStore) List() []LogSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogSummary, 0, len(s.byID))
	for _, e := range s.byID {
		out = append(out, LogSummary{
			ID:         e.ID,
			Version:    e.Version,
			Platform:   e.Platform,
			Arch:       e.Arch,
			Level:      e.Level,
			Lines:      len(e.Lines),
			Bytes:      e.Bytes,
			ReceivedAt: e.ReceivedAt,
			UpdatedAt:  e.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Clear forgets one machine's report. It is what the panel's forget button
// does, and what a rotated credential should be followed by.
func (s *LogStore) Clear(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	delete(s.byID, id)
	return ok
}

// Text renders a report as the log file the machine wrote: one line each, no
// extra decoration. It is what the download link serves.
func (e LogEntry) Text() string {
	return strings.Join(e.Lines, "\n")
}
