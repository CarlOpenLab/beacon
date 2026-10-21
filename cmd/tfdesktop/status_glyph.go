package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
)

// statusGlyph draws what a session is doing, as the sidebar does it: a ring
// turning while it works, a filled dot when it needs the user, a green dot for
// work finished unseen, a red dot for a failure, and a quiet ring for a shell
// that ended. A shell at its prompt, and one whose finished work has been seen,
// draw nothing: a row at rest says nothing rather than wearing a checkmark that
// means "nothing".
//
// The states are the daemon's (internal/status); a row without one — a session
// the list has not described yet — draws nothing.
func statusGlyph(c *ui.Context, pal *uitokens.Palette, state string, unread bool, progress int, size float32) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0)
	switch state {
	case "working":
		box.Label("working")
	case "waiting":
		box.Label("needs you")
	case "error":
		box.Label("failed")
	case "exited":
		box.Label("exited")
	case "idle":
		if unread {
			box.Label("done")
		}
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		rad := min(r.W, r.H)/2 - 1
		dot := rad * 0.68
		switch {
		case state == "working" && progress >= 0:
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.Text.Alpha(0.13))
			// From the top, clockwise: the circle starts at 3 o'clock.
			p.Transform(ui.Rotation(-90), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, max(0.02, float32(progress)/100)), 1.7, pal.Text)
			})
		case state == "working":
			var track ui.Path
			track.Circle(cx, cy, rad)
			p.StrokePath(&track, 1.7, pal.Text.Alpha(0.13))
			p.AnimationFrame()
			deg := float32(p.Now().UnixMilli()%1000) / 1000 * 360
			p.Transform(ui.Rotation(deg), cx, cy, func() {
				var arc ui.Path
				arc.Circle(cx, cy, rad)
				p.StrokePath(arc.Trim(0, 0.3), 1.7, pal.Text)
			})
		case state == "waiting":
			var disc ui.Path
			disc.Circle(cx, cy, dot)
			p.FillPath(&disc, pal.Attention)
		case state == "error":
			var disc ui.Path
			disc.Circle(cx, cy, dot)
			p.FillPath(&disc, pal.Danger)
			p.Line(cx, cy-dot*0.42, cx, cy+dot*0.12, 1.4, pal.OnDone)
			var point ui.Path
			point.Circle(cx, cy+dot*0.46, 0.9)
			p.FillPath(&point, pal.OnDone)
		case state == "exited":
			var ring ui.Path
			ring.Circle(cx, cy, dot)
			p.StrokePath(&ring, 1.4, pal.TextFaint)
		case state == "idle" && unread:
			var disc ui.Path
			disc.Circle(cx, cy, dot)
			p.FillPath(&disc, pal.Success)
		}
	})
	return box
}

// ago is how long ago a time in Unix milliseconds was, shortly: the relative
// time shown at the end of a row. It is recomputed by the frame that draws
// it, and the sidebar asks for another frame every 20 seconds.
func ago(ms int64, now time.Time) string {
	if ms == 0 {
		return ""
	}
	d := now.Sub(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return itoa(int(d/time.Hour)) + "h"
	case d < 7*24*time.Hour:
		return itoa(int(d/(24*time.Hour))) + "d"
	}
	return itoa(int(d/(7*24*time.Hour))) + "w"
}
