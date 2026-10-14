package relay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DeviceEntry is one enrolled machine as the admin UI and the CLI see it.
type DeviceEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	AddedAt string `json:"addedAt,omitempty"`
}

// Registry is the member registry: the devices.json file, plus an in-memory
// mirror guarded by a mutex.
//
// The relay serves out of this rather than out of a snapshot taken at startup
// because machines are now enrolled from the admin page while the relay runs. A
// credential minted there has to be accepted on the very next dial, not after a
// restart. Every mutation writes the file first and only then swaps the mirror,
// so on-disk and in-memory state cannot drift apart after a failed write.
type Registry struct {
	path string

	mu      sync.RWMutex
	devices map[string]DeviceRecord
}

// OpenRegistry loads the registry at path, creating an empty one if the file is
// missing. An empty registry is legal: it is the state of a relay whose operator
// has not enrolled anything yet, and the admin setup flow needs exactly that.
func OpenRegistry(path string) (*Registry, error) {
	g := &Registry{path: path, devices: map[string]DeviceRecord{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return g, nil
		}
		return nil, err
	}
	var f DevicesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for id, rec := range f.Devices {
		if rec.Secret == "" {
			return nil, fmt.Errorf("%s: device %q has no secret", path, id)
		}
		g.devices[id] = rec
	}
	return g, nil
}

// Path is where this registry is persisted.
func (g *Registry) Path() string { return g.path }

// Secret returns the credential for id, and whether it is known at all.
func (g *Registry) Secret(id string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	rec, ok := g.devices[id]
	if !ok {
		return "", false
	}
	return rec.Secret, true
}

// Len reports how many machines are enrolled.
func (g *Registry) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.devices)
}

// Entries lists every enrolled machine, sorted by id so the admin table does not
// reshuffle between polls.
func (g *Registry) Entries() []DeviceEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]DeviceEntry, 0, len(g.devices))
	for id, rec := range g.devices {
		out = append(out, DeviceEntry{ID: id, Name: rec.Name, AddedAt: rec.AddedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Enroll mints a credential for one machine and appends it to the registry.
// The secret is returned once; only its stored copy can be read back.
func (g *Registry) Enroll(name, now string) (id, secret string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	next := make(map[string]DeviceRecord, len(g.devices)+1)
	for k, v := range g.devices {
		next[k] = v
	}
	id = uniqueID(next, slug(name))
	secret, err = newSecret()
	if err != nil {
		return "", "", err
	}
	next[id] = DeviceRecord{Secret: secret, Name: name, AddedAt: now}
	if err := g.write(next); err != nil {
		return "", "", err
	}
	g.devices = next
	return id, secret, nil
}

// Revoke removes a machine's credential. Revoking an unknown id is not an
// error: the admin table may be a poll behind.
func (g *Registry) Revoke(id string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.devices[id]; !ok {
		return false, nil
	}
	next := make(map[string]DeviceRecord, len(g.devices))
	for k, v := range g.devices {
		if k != id {
			next[k] = v
		}
	}
	if err := g.write(next); err != nil {
		return false, err
	}
	g.devices = next
	return true, nil
}

// write persists a device map atomically: temp file in the same directory, then
// rename. The file is 0600 because it is a list of machine credentials.
func (g *Registry) write(devices map[string]DeviceRecord) error {
	if g.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(DevicesFile{Devices: devices}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.path), 0o700); err != nil {
		return err
	}
	tmp := g.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, g.path)
}

// inMemoryRegistry builds a registry that is never written to, for a relay
// configured with a fixed device map (tests, and the -devices flag path).
func inMemoryRegistry(devices map[string]string) *Registry {
	g := &Registry{devices: map[string]DeviceRecord{}}
	for id, secret := range devices {
		g.devices[id] = DeviceRecord{Secret: secret}
	}
	return g
}
