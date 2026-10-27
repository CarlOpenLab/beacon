package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/internal/protocol"
	"github.com/CarlOpenLab/beacon/internal/tunnel"
)

// TestRenderRailPreviews writes the machine rail with nine machines, one in
// each state, at twice the pixel density, in both appearances. At 1x a stroke
// is rounded to whole pixels, so the weight tiers only show at 2x.
func TestRenderRailPreviews(t *testing.T) {
	dir := os.Getenv("TF_RENDER")
	if dir == "" {
		t.Skip("set TF_RENDER=<dir> to write the rail previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	states := []struct {
		id, status string
		online     bool
		unread     bool
	}{
		{"mac-mini", "", true, false},
		{"macbook", "working", true, false},
		{"db-main", "idle", true, true},
		{"aws-eu", "waiting", true, false},
		{"k8s-node", "", true, false},
		{"nas-1", "", true, false},
		{"pi-3", "", true, false},
		{"edge-router", "", true, false},
		{"print-hub", "", false, false},
		{"web-server", "", true, false},
		{"win-2024", "error", true, false},
	}
	for _, mode := range []string{themeLight, themeDark} {
		t.Run(mode, func(t *testing.T) {
			w := previewWindow(t, mode, true)
			w.roster = nil
			w.remoteSessions = map[string][]protocol.Summary{}
			for _, s := range states {
				w.roster = append(w.roster, tunnel.Machine{ID: s.id, Name: s.id,
					Host: protocol.Host{Hostname: s.id + ".local", Platform: "darwin"}, Online: s.online})
				if s.status != "" {
					w.remoteSessions[s.id] = []protocol.Summary{{SessionID: s.id + "-1", Title: "x", Alive: true,
						Status: s.status, Unread: s.unread}}
				}
			}
			for _, m := range w.railMachines(time.Now()) {
				t.Logf("%s icon=%s state=%q", m.ID, w.machineIconKey(m), w.railDotState(m))
			}
			tt := ui.NewTester(func(c *ui.Context) { w.view(c) }, previewWidth, previewHeight)
			tt.SetScale(2)
			tt.Frame()
			if os.Getenv("TF_RAIL_END") != "" {
				// Choosing the last machine makes the strip bring it into view, as
				// a person's click does: the scroll is the app's own.
				w.machineSel = "win-2024"
				for i := 0; i < 4; i++ {
					tt.Frame()
					tt.Advance(100 * time.Millisecond)
				}
				t.Logf("X=%v max=%v", w.railScroll.X, w.railScroll.MaxX)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("rail-%s%s%s@2x.png", mode, os.Getenv("TF_RAIL_END"), os.Getenv("TF_RAIL_HOVER"))))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
