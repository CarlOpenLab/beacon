// Package fonts lists the font families installed on this machine.
//
// The terminal lets the user pick a family by name, so the picker has to
// show what is actually installed: a hand-written list cannot know about
// the variants people install (Nerd Font builds, for instance, are named
// after the family they patch).
package fonts

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font/sfnt"
)

// Dirs returns the directories that hold installed fonts on this platform,
// keeping only those that exist.
func Dirs() []string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		candidates = []string{
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
			"/Library/Fonts",
			filepath.Join(home, "Library/Fonts"),
		}
	case "windows":
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		candidates = []string{filepath.Join(windir, "Fonts")}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, filepath.Join(local, `Microsoft\Windows\Fonts`))
		}
	default:
		home, _ := os.UserHomeDir()
		candidates = []string{
			"/usr/share/fonts",
			"/usr/local/share/fonts",
			filepath.Join(home, ".fonts"),
			filepath.Join(home, ".local/share/fonts"),
		}
	}

	var dirs []string
	seen := make(map[string]bool)
	for _, dir := range candidates {
		if dir == "" || seen[dir] {
			continue
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	return dirs
}

var (
	familiesOnce  sync.Once
	familiesCache []string
)

// Families returns the font families installed on this machine, sorted and
// without duplicates. The scan walks Dirs once and is cached.
func Families() []string {
	familiesOnce.Do(func() {
		familiesCache = FamiliesIn(Dirs())
	})
	return familiesCache
}

// FamiliesIn returns the font families found in dirs, sorted and without
// duplicates. Directories are searched recursively, because Linux font
// trees are nested.
func FamiliesIn(dirs []string) []string {
	found := make(map[string]bool)
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				// An unreadable file or directory is not fatal: keep the
				// families we can read.
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || !isFontFile(path) {
				return nil
			}
			for _, family := range familiesOfFile(path) {
				found[family] = true
			}
			return nil
		})
	}

	families := make([]string, 0, len(found))
	for family := range found {
		families = append(families, family)
	}
	sort.Strings(families)
	return families
}

func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc", ".otc", ".dfont":
		return true
	}
	return false
}

// familiesOfFile reads the family names declared inside one font file. A
// file that cannot be parsed yields nothing: a broken font must not keep
// the rest of the list from loading.
func familiesOfFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var fonts []*sfnt.Font
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttc", ".otc", ".dfont":
		collection, err := sfnt.ParseCollectionReaderAt(file)
		if err != nil {
			// Some files with these extensions are plain single fonts.
			if single, err := sfnt.ParseReaderAt(file); err == nil {
				fonts = append(fonts, single)
			}
			break
		}
		for i := 0; i < collection.NumFonts(); i++ {
			if font, err := collection.Font(i); err == nil {
				fonts = append(fonts, font)
			}
		}
	default:
		if single, err := sfnt.ParseReaderAt(file); err == nil {
			fonts = append(fonts, single)
		}
	}

	var buffer sfnt.Buffer
	var out []string
	for _, font := range fonts {
		if family := familyOf(font, &buffer); family != "" {
			out = append(out, family)
		}
	}
	return out
}

// familyOf prefers the typographic family, which is the one that groups
// the styles of a family ("JetBrains Mono" rather than "JetBrains Mono
// Bold"). The legacy family is the fallback.
func familyOf(font *sfnt.Font, buffer *sfnt.Buffer) string {
	for _, id := range []sfnt.NameID{sfnt.NameIDTypographicFamily, sfnt.NameIDFamily} {
		name, err := font.Name(buffer, id)
		if err != nil {
			continue
		}
		if name = strings.TrimSpace(name); name != "" {
			return name
		}
	}
	return ""
}
