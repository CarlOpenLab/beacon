// Package settings stores the desktop shell's preferences on disk.
//
// Everything the settings window changes lives here: the terminal's font, the
// relay this machine joins, and whether the local service that serves browsers
// is on. A Store is safe for concurrent use, because the window's UI thread and
// the goroutines that carry out a change do not run on the same schedule.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// DefaultFontSize is the terminal size the settings window offers on a first
// run, in DIPs.
const DefaultFontSize = 14

// Settings are the desktop shell's persisted preferences. The zero value is
// what a first run gets: the system font at the plugin's own default size, no
// relay, and the local service on.
type Settings struct {
	// FontFamily is the terminal's font family list. Empty means the system's
	// monospaced font.
	FontFamily string `json:"fontFamily,omitempty"`
	// FontSize is the terminal's size in DIPs. Zero keeps the plugin's default.
	FontSize float64 `json:"fontSize,omitempty"`

	// RelayURL is the relay's member endpoint, such as
	// wss://relay.example/relay/agent. Empty disables the link.
	RelayURL string `json:"relayUrl,omitempty"`
	// DeviceID and DeviceSecret are this machine's relay credential.
	DeviceID     string `json:"deviceId,omitempty"`
	DeviceSecret string `json:"deviceSecret,omitempty"`
	// RelayOn asks the shell to keep a link to RelayURL while it runs.
	RelayOn bool `json:"relayOn,omitempty"`

	// ServerOff turns off the local HTTP service that serves the browser
	// client. False — the value a first run and a missing key both give — keeps
	// it on, which is what a desktop shell normally wants.
	ServerOff bool `json:"serverOff,omitempty"`
}

// Dir is where the desktop shell keeps its settings. TERM_FLEET_CONFIG_HOME
// overrides it, which is how tests and a portable copy keep their own file.
func Dir() string {
	if d := os.Getenv("TERM_FLEET_CONFIG_HOME"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "term-fleet")
}

// Path is the settings file inside dir.
func Path(dir string) string { return filepath.Join(dir, "settings.json") }

// Load reads the settings in dir. A missing or damaged file is not an error:
// both yield the zero Settings, so a half-written file can never keep the shell
// from starting.
func Load(dir string) Settings {
	b, err := os.ReadFile(Path(dir))
	if err != nil {
		return Settings{}
	}
	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}
	}
	return s
}

// Save writes the settings to dir, replacing the file atomically so a crash
// mid-write leaves the previous settings intact rather than a truncated file.
func Save(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(dir, "settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename below has succeeded

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	return os.Rename(name, Path(dir))
}

// Store is Settings kept in one file, safe for concurrent use.
type Store struct {
	mu  sync.Mutex
	dir string
	cur Settings
}

// Open loads the settings in dir into a store. It never fails; a first run
// starts from the zero Settings.
func Open(dir string) *Store {
	return &Store{dir: dir, cur: Load(dir)}
}

// Dir is the directory the store writes to.
func (s *Store) Dir() string { return s.dir }

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Update applies fn to the current settings and writes the result. It returns
// the settings as stored, so the caller can apply exactly what was persisted —
// and the old ones, on the rare write that fails.
func (s *Store) Update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	fn(&next)
	if err := Save(s.dir, next); err != nil {
		return s.cur, err
	}
	s.cur = next
	return next, nil
}
