// The session inspector: the facts about the shell on screen, in a column beside
// it, for the shell that is worth reading rather than driving.
//
// The browser client keeps the same column, at the same width, and it is the one
// part of the window that is about a single shell rather than about the window:
// its id, where it is running, how big it is, who is typing into it. A shell
// driven through a relay is exactly the one whose facts are worth having on
// screen, so the column is open on request and closed by default — a window is
// 800px wide before it has been resized, and a 280px column over a terminal is a
// terminal cut to 500px.
//
// It shows only what the hub has actually said. A shell the window has asked
// for and not been told about has no id yet, and the panel says so rather than
// showing a blank.

package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// inspectWidth is the browser client's --w-inspect. The two clients show the
// same fields about the same shell, in the same order, at the same width.
const inspectWidth = 280

// inspectVisible is whether the column is open. A window is 800px wide before
// it has been resized, and a 280px column over a terminal is a terminal cut to
// 500px, so the column starts closed and opens on request.
var inspectVisible = false

// inspectField is one fact about the shell, as the browser client's rows are:
// a label, its value, and a note on the value where the value alone is not the
// whole story.
type inspectField struct {
	label string
	value string
	note  string
}

// inspectFields is what the shell on screen can be read for. The order is the
// browser client's, so the same shell reads the same way in both.
func (w *window) inspectFields() []inspectField {
	tb := w.activeTab()
	if tb == nil {
		return nil
	}
	out := []inspectField{
		{label: "会话", value: or(tb.id, "待创建")},
		{label: "机器", value: w.machineName(tb.machine)},
	}
	if tb.term != nil {
		cols, rows := tb.term.Size()
		out = append(out, inspectField{label: "尺寸", value: fmt.Sprintf("%d×%d", cols, rows)})
	}
	// The lease is the one fact that explains everything else on screen: why
	// the composer will not take a keystroke, why the pane is marked 只读, why
	// the banner is up there saying so.
	switch {
	case !tb.alive:
		out = append(out, inspectField{label: "状态", value: "已退出", note: "会话已结束"})
	case tb.held():
		out = append(out, inspectField{label: "状态", value: "控制中", note: "本端"})
	default:
		out = append(out, inspectField{label: "状态", value: "只读", note: tb.controller})
	}
	out = append(out,
		inspectField{label: "程序", value: or(tb.program, "—")},
		inspectField{label: "退出码", value: exitText(tb)},
	)
	return out
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func exitText(tb *tab) string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.done {
		return "—"
	}
	return fmt.Sprintf("%d", tb.exit)
}

// inspectPanel is the column itself. Its head names the shell, its body lists
// the facts, and its head carries the button that closes it again.
func (w *window) inspectPanel(c *ui.Context) {
	if !inspectVisible {
		return
	}
	t := c.Theme()
	col := shellColorsFor(t)

	ui.Column(c).
		Width(inspectWidth).FillHeight().
		Background(col.pane).
		BorderWidth(1, 0, 0, 0).BorderColor(col.hairline).
		Children(func() {
			// The head is the panel's title row, as tall as the band at the top
			// of the window, with the close button beside the title the way the
			// browser client's own toggle sits.
			ui.Row(c).Height(headerHeight).PaddingX(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "会话检视").FontSize(12).FontWeight(600)
				ui.Spacer(c)
				hide := ui.Box(c).
					Size(22, 22).Radius(radiusSmall).
					AlignItems(ui.Center).Justify(ui.Center).
					Label("收起会话检视").Tooltip("收起会话检视")
				if hide.Hovered() {
					hide.Background(t.SurfaceHover)
				}
				hide.Children(func() {
					ui.Icon(c, iconClose).FontSize(11).TextColor(col.icon)
				})
				if hide.Clicked() {
					inspectVisible = false
					w.invalidate()
				}
			})

			w.inspectBody(c)
		})
}

// inspectBody lists the facts, newest information first: the role, then the
// numbers. A shell that is not this window's is still readable — that is the
// point of the column.
func (w *window) inspectBody(c *ui.Context) {
	t := c.Theme()
	tb := w.activeTab()
	if tb == nil {
		return
	}

	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(12).Gap(14).Children(func() {
			// The role is the first thing on the column, stated as a chip rather
			// than a row: it is the fact that changes everything else.
			role, roleColor := inspectRole(t, tb)
			chip := ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(6, 10).Radius(radius).
				Background(roleColor.Alpha(0.10)).
				BorderWidth(1).BorderColor(roleColor.Alpha(0.42))
			chip.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(roleColor)
				ui.Text(c, role).FontSize(12).FontWeight(600).TextColor(roleColor)
			})

			// The facts, as a definition list: the label above the value, the
			// way the browser client's rows read.
			ui.Column(c).Gap(12).Children(func() {
				for _, f := range w.inspectFields() {
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, f.label).FontSize(10.5).TextColor(shellColorsFor(t).quiet)
						value := f.value
						if f.note != "" {
							value = fmt.Sprintf("%s · %s", f.value, f.note)
						}
						ui.Text(c, value).FontSize(12).TextColor(t.Text).SingleLine()
					})
				}
			})
		})
	})
}

// inspectRole is the shell's role in words and in colour: green while this window
// drives it, amber while someone else does, red once it is gone.
func inspectRole(t *ui.Theme, tb *tab) (string, ui.Color) {
	switch {
	case !tb.alive:
		return "已退出", t.Danger
	case tb.held():
		return "控制中", t.Success
	default:
		return "只读", t.Warning
	}
}
