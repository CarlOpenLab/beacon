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
// run, in DIPs. It matches the terminal plugin's own default (13), so the
// number the window shows is the one the terminal is actually using.
const DefaultFontSize = 13

// Settings are the desktop shell's persisted preferences. The zero value is
// what a first run gets: the system font at the plugin's own default size, no
// relay, and the local service on.
type Settings struct {
	// FontFamily is the terminal's font family list. Empty means the system's
	// monospaced font.
	FontFamily string `json:"fontFamily,omitempty"`
	// FontSize is the terminal's size in DIPs. Zero keeps the plugin's default,
	// which is DefaultFontSize.
	FontSize float64 `json:"fontSize,omitempty"`

	// RelayURL is the relay's member endpoint, such as
	// wss://relay.example/relay/agent. Empty disables the link.
	RelayURL string `json:"relayUrl,omitempty"`
	// DeviceID and DeviceSecret are this machine's relay credential.
	DeviceID     string `json:"deviceId,omitempty"`
	DeviceSecret string `json:"deviceSecret,omitempty"`
	// RelayCA is a PEM file holding the certificate that signed the relay's
	// TLS certificate. It is how a desktop shell trusts a relay behind a
	// private CA or a self-signed certificate, without installing it
	// system-wide. Empty means "use the system roots".
	RelayCA string `json:"relayCA,omitempty"`
	// RelayOn asks the shell to keep a link to RelayURL while it runs.
	RelayOn bool `json:"relayOn,omitempty"`

	// ServerOff turns off the local HTTP service that serves the browser
	// client. False — the value a first run and a missing key both give — keeps
	// it on, which is what a desktop shell normally wants.
	ServerOff bool `json:"serverOff,omitempty"`

	// ControlOff refuses other clients — a browser, a phone, another machine —
	// the control of this machine's shells: they may watch, and that is all.
	// False keeps the normal answer, which is that joining a relay is the consent
	// to be driven from elsewhere. The machine's own window is never refused,
	// because a switch on a machine must not lock the machine out of itself.
	ControlOff bool `json:"controlOff,omitempty"`

	// Theme is the desktop shell's appearance: "light" or "dark". Empty — what a
	// first run gets — follows the system.
	Theme string `json:"theme,omitempty"`

	// Background is the window's wash: "rex", "mist", "meadow", "dusk" or
	// "plain". Empty — what a first run and a missing key both give — is rex,
	// the pink lavender one.
	Background string `json:"background,omitempty"`

	// The four notifications below are on unless they are turned off, so each
	// field names what turns it off rather than what turns it on: a first run,
	// and a settings file that predates the field, notifies.
	//
	// NotifyWaitingOff stops the notification a session sends when it comes to
	// need the user, NotifyDoneOff the one an agent sends when it finishes, and
	// NotifyErrorOff the one a program sends when it reports a failure.
	NotifyWaitingOff bool `json:"notifyWaitingOff,omitempty"`
	NotifyDoneOff    bool `json:"notifyDoneOff,omitempty"`
	NotifyErrorOff   bool `json:"notifyErrorOff,omitempty"`
	// DockBadgeOff hides how many sessions need the user from the Dock icon.
	DockBadgeOff bool `json:"dockBadgeOff,omitempty"`

	// Keybinds are the shortcuts the user changed, by the action id the window
	// names them with: "session.new": "cmd+t". An accelerator the file holds
	// wins over the action's default, and an empty one unbinds it — which is why
	// the map's keys are consulted, not only its values.
	Keybinds map[string]string `json:"keybinds,omitempty"`

	// Language is the interface language, as a BCP 47 tag of the languages the
	// app is written in: "zh-Hans" or "en". Empty — what a first run gets —
	// follows the languages the system says the person reads.
	Language string `json:"language,omitempty"`
}

// Dir is where the desktop shell keeps its settings. BEACON_CONFIG_HOME or
// TERM_FLEET_CONFIG_HOME overrides it, which is how tests and a portable copy keep their own file.
func Dir() string {
	if d := os.Getenv("BEACON_CONFIG_HOME"); d != "" {
		return d
	}
	if d := os.Getenv("TERM_FLEET_CONFIG_HOME"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	beaconDir := filepath.Join(base, "beacon")
	if _, err := os.Stat(beaconDir); err == nil {
		return beaconDir
	}
	legacyDir := filepath.Join(base, "term-fleet")
	if _, err := os.Stat(legacyDir); err == nil {
		return legacyDir
	}
	return beaconDir
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
