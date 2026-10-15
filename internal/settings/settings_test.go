package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestLoadMissingIsZero(t *testing.T) {
	dir := t.TempDir()
	got := Load(dir)
	if got != (Settings{}) {
		t.Fatalf("Load(%q) = %+v, want zero Settings", dir, got)
	}
	// Zero Settings is the first-run contract: the local service is on.
	if got.ServerOff {
		t.Fatal("zero Settings wants the local service on, got ServerOff")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Settings{
		FontFamily:   "JetBrains Mono",
		FontSize:     15,
		RelayURL:     "wss://relay.example/relay/agent",
		DeviceID:     "mac-mini",
		DeviceSecret: "hunter2",
		RelayOn:      true,
		ServerOff:    true,
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := Load(dir); got != want {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Settings{FontSize: 13}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v, want only settings.json", names)
	}
}

func TestLoadDamagedFileIsZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(Path(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := Load(dir); got != (Settings{}) {
		t.Fatalf("Load of a damaged file = %+v, want zero Settings", got)
	}
	// A damaged file must not stop the next write from succeeding.
	if err := Save(dir, Settings{FontSize: 16}); err != nil {
		t.Fatalf("Save after damage: %v", err)
	}
	if got := Load(dir).FontSize; got != 16 {
		t.Fatalf("FontSize after rewrite = %v, want 16", got)
	}
}

func TestSavePreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Settings{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(Path(dir))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Windows has no Unix permission bits: every writable file reports 0666
	// there, so the mode only means something where those bits exist.
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("settings file mode = %o, want 600", mode)
		}
	}
}

func TestStoreUpdatePersistsAndReturnsStored(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	got, err := st.Update(func(s *Settings) { s.FontSize = 18 })
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.FontSize != 18 {
		t.Fatalf("Update returned %+v, want FontSize 18", got)
	}
	if st.Get().FontSize != 18 {
		t.Fatalf("Get after Update = %+v, want FontSize 18", st.Get())
	}
	// A fresh store sees the same value, so the write reached the disk.
	if reread := Open(dir).Get().FontSize; reread != 18 {
		t.Fatalf("reopened store = %v, want FontSize 18", reread)
	}
}

func TestStoreUpdateKeepsPreviousValueOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	if _, err := st.Update(func(s *Settings) { s.FontSize = 12 }); err != nil {
		t.Fatalf("first Update: %v", err)
	}
	// Point the store at a path whose parent is a file, so the next write
	// cannot create its directory.
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	st.dir = blocked
	got, err := st.Update(func(s *Settings) { s.FontSize = 99 })
	if err == nil {
		t.Fatal("Update into an unusable directory succeeded, want an error")
	}
	if got.FontSize != 12 || st.Get().FontSize != 12 {
		t.Fatalf("after a failed write = %+v, want the previous value 12", got)
	}
}

func TestStoreConcurrentUpdates(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if _, err := st.Update(func(s *Settings) { s.FontSize = float64(n + 1) }); err != nil {
				t.Errorf("Update: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if got := st.Get().FontSize; got < 1 || got > 20 {
		t.Fatalf("stored FontSize = %v, want one of the written values", got)
	}
	if Load(dir).FontSize != st.Get().FontSize {
		t.Fatalf("file and store disagree: %v vs %v", Load(dir).FontSize, st.Get().FontSize)
	}
}

func TestDirHonorsOverride(t *testing.T) {
	t.Setenv("TERM_FLEET_CONFIG_HOME", "/tmp/tf-test-config")
	if got := Dir(); got != "/tmp/tf-test-config" {
		t.Fatalf("Dir() = %q, want the override", got)
	}
}
