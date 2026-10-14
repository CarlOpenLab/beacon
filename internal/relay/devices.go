package relay

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// DeviceRecord is one enrolled member as stored on disk.
//
// Name is kept only so an operator reading the file knows what a row is for;
// the authoritative name is what the member reports in its hello, because a
// machine may be renamed without touching the relay.
type DeviceRecord struct {
	Secret  string `json:"secret"`
	Name    string `json:"name,omitempty"`
	AddedAt string `json:"addedAt,omitempty"`
}

// DevicesFile is the on-disk shape of the member registry.
type DevicesFile struct {
	Devices map[string]DeviceRecord `json:"devices"`
}

// LoadDevices reads the member registry, returning id -> secret.
func LoadDevices(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no device registry at %s: run `tfrelay enroll -name <machine>` first", path)
		}
		return nil, err
	}
	var f DevicesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.Devices) == 0 {
		return nil, fmt.Errorf("%s lists no devices: run `tfrelay enroll -name <machine>`", path)
	}
	out := make(map[string]string, len(f.Devices))
	for id, rec := range f.Devices {
		if rec.Secret == "" {
			return nil, fmt.Errorf("%s: device %q has no secret", path, id)
		}
		out[id] = rec.Secret
	}
	return out, nil
}

// EnrollDevice mints a credential for one machine and appends it to the
// registry. The secret is returned once, to be pasted into that machine's
// agent configuration; the registry itself is written 0600.
func EnrollDevice(path, name string, now string) (id, secret string, err error) {
	f := DevicesFile{Devices: map[string]DeviceRecord{}}
	if b, rerr := os.ReadFile(path); rerr == nil {
		if uerr := json.Unmarshal(b, &f); uerr != nil {
			return "", "", fmt.Errorf("%s: %w", path, uerr)
		}
		if f.Devices == nil {
			f.Devices = map[string]DeviceRecord{}
		}
	} else if !os.IsNotExist(rerr) {
		return "", "", rerr
	}

	id = uniqueID(f.Devices, slug(name))
	secret, err = newSecret()
	if err != nil {
		return "", "", err
	}
	f.Devices[id] = DeviceRecord{Secret: secret, Name: name, AddedAt: now}

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return "", "", err
	}
	return id, secret, os.Rename(tmp, path)
}

// ListDevices reports enrolled ids and names, sorted, for operator output.
func ListDevices(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f DevicesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.Devices))
	for id, rec := range f.Devices {
		if rec.Name != "" {
			out = append(out, id+" ("+rec.Name+")")
		} else {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// newSecret returns 32 random bytes, base64url-encoded with no padding.
func newSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// slug turns a machine name into a readable id prefix.
func slug(name string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
			prevDash = false
		case !prevDash && sb.Len() > 0:
			sb.WriteByte('-')
			prevDash = true
		}
	}
	s := strings.Trim(sb.String(), "-")
	if s == "" {
		s = "machine"
	}
	if len(s) > 32 {
		s = strings.Trim(s[:32], "-")
	}
	return s
}

// uniqueID appends a short random suffix so two machines with the same name
// still get distinct identities.
func uniqueID(existing map[string]DeviceRecord, base string) string {
	for i := 0; i < 64; i++ {
		var b [3]byte
		if _, err := rand.Read(b[:]); err != nil {
			continue
		}
		id := base + "-" + hex.EncodeToString(b[:])
		if _, clash := existing[id]; !clash {
			return id
		}
	}
	return base // astronomically unlikely; better a duplicate than no id
}