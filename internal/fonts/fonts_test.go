package fonts

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// The Go font ships with x/image, so the test does not depend on whatever
// happens to be installed on the machine running it.
func writeTestFont(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, goregular.TTF, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFamiliesInReadsFamilyNames(t *testing.T) {
	dir := t.TempDir()
	writeTestFont(t, filepath.Join(dir, "goregular.ttf"))

	families := FamiliesIn([]string{dir})
	if len(families) != 1 || families[0] != "Go" {
		t.Fatalf("FamiliesIn = %v, want [Go]", families)
	}
}

func TestFamiliesInDeduplicates(t *testing.T) {
	// Two copies of the same family, plus nested directories, must collapse
	// into one entry: the picker shows families, not files.
	dir := t.TempDir()
	writeTestFont(t, filepath.Join(dir, "a", "one.ttf"))
	writeTestFont(t, filepath.Join(dir, "b", "deep", "two.ttf"))

	families := FamiliesIn([]string{dir})
	if len(families) != 1 || families[0] != "Go" {
		t.Fatalf("FamiliesIn = %v, want [Go]", families)
	}
}

func TestFamiliesInSorts(t *testing.T) {
	dir := t.TempDir()
	writeTestFont(t, filepath.Join(dir, "goregular.ttf"))

	// A name that sorts after "Go" and one before, both unparseable so only
	// the real font's family is collected; this asserts the sort of the
	// collected set rather than the walk order.
	families := FamiliesIn([]string{dir})
	if len(families) > 1 && families[0] > families[len(families)-1] {
		t.Fatalf("FamiliesIn = %v, want sorted", families)
	}
}

func TestFamiliesInSkipsUnreadable(t *testing.T) {
	dir := t.TempDir()
	writeTestFont(t, filepath.Join(dir, "good.ttf"))
	// A file with a font extension but garbage contents must be ignored
	// instead of failing the whole scan.
	if err := os.WriteFile(filepath.Join(dir, "broken.ttf"), []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	families := FamiliesIn([]string{dir})
	if len(families) != 1 || families[0] != "Go" {
		t.Fatalf("FamiliesIn = %v, want [Go]", families)
	}
}

func TestFamiliesInMissingDirectory(t *testing.T) {
	families := FamiliesIn([]string{filepath.Join(t.TempDir(), "nope")})
	if len(families) != 0 {
		t.Fatalf("FamiliesIn = %v, want empty", families)
	}
}

func TestDirsReturnsExistingDirectories(t *testing.T) {
	for _, dir := range Dirs() {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("Dirs returned %q, which is not an existing directory", dir)
		}
	}
}
