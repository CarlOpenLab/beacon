// The command bar: the browser client's composer, in this window's own terms.
//
// B · Command Deck's input is not the terminal. A person types a command into a
// composer with a prompt glyph, a recallable history above it, and a button
// that says what the keystroke does. The terminal shows what the shell said.
//
// This matters beyond looks. The terminal is a display surface: it cannot
// produce Esc, Tab, Ctrl, or an arrow key, because those are what the shell
// reads a text box for. Typing straight into the terminal left this window with
// no way to send any of them, and no way to recall a command already sent.

package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

// commandBarHeight is what the composer adds over the terminal. The browser
// client's is two lines tall — the box and the button beside it — so a command
// fits without the box scrolling.
const commandBarHeight = 56

// historyBarHeight is the row of chips above the composer. It only appears
// once something has been sent, so a window that has sent nothing spends none
// of its height on it.
const historyBarHeight = 30

// historyMost is how many chips the bar keeps. The browser client shows the
// last twelve, newest first, which is what fits on one row without scrolling.
const historyMost = 12

// commandBar is the window's own record of what it has typed, and the box it is
// typed into again.
type commandBar struct {
	// draft is what is in the box now, which is not the same as the shell's
	// prompt: a person may type a whole line before sending it.
	draft string
	// sent is the same list the active tab keeps, so the bar follows the tab
	// rather than the window.
	sent []string
	// recall is where the up and down arrows have walked to in sent; len(sent)
	// means the box is showing the draft again.
	recall int
}

// commandBarFor returns the window's composer, making it if the window was
// built without one — a test window, or one that predates the bar.
func (w *window) commandBarFor() *commandBar {
	if w.cmdBar == nil {
		w.cmdBar = newCommandBar()
	}
	return w.cmdBar
}

func newCommandBar() *commandBar {
	return &commandBar{recall: 0}
}

// push records a sent command, newest last, and leaves recall past the end —
// which is where the browser client puts it too.
func (b *commandBar) push(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// A repeated command is not new history: pressing up twice to run the same
	// command again should not make two entries.
	if n := len(b.sent); n > 0 && b.sent[n-1] == text {
		b.recall = n
		return
	}
	b.sent = append(b.sent, text)
	if len(b.sent) > historyMost {
		b.sent = b.sent[len(b.sent)-historyMost:]
	}
	b.recall = len(b.sent)
}

// up walks toward older commands; the one past the newest restores the draft.
func (b *commandBar) up(draft string) string {
	if b.recall > 0 {
		b.recall--
	}
	if b.recall < len(b.sent) {
		return b.sent[b.recall]
	}
	return draft
}

// down walks back toward newer ones, and past the newest restores the draft.
func (b *commandBar) down(draft string) string {
	if b.recall < len(b.sent) {
		b.recall++
	}
	if b.recall < len(b.sent) {
		return b.sent[b.recall]
	}
	return draft
}

// track points the bar at the active tab's history, so switching tabs brings
// back what was sent there rather than showing another shell's commands.
func (b *commandBar) track(tb *tab) {
	if tb == nil {
		b.sent, b.recall = nil, 0
		return
	}
	b.sent = tb.sent
	b.recall = len(tb.sent)
}

// shown is the history as the bar draws it: newest first, at most one row.
func (b *commandBar) shown() []string {
	if b == nil || len(b.sent) == 0 {
		return nil
	}
	out := make([]string, 0, len(b.sent))
	for i := len(b.sent) - 1; i >= 0; i-- {
		out = append(out, b.sent[i])
	}
	return out
}

// historyBar is the row of chips above the composer: what has been sent, newest
// first, each one putting itself back in the box when clicked.
func (w *window) historyBar(c *ui.Context) {
	b := w.commandBarFor()
	if len(b.shown()) == 0 {
		return
	}
	t := c.Theme()

	ui.Row(c).
		Height(historyBarHeight).PaddingX(14).Gap(6).
		AlignItems(ui.Center).
		Background(shellColorsFor(t).pane).
		BorderWidth(1, 0, 0, 0).BorderColor(shellColorsFor(t).hairline).
		Children(func() {
			ui.Text(c, "历史").FontSize(11).TextColor(t.TextMuted)
			ui.Scroll(c).Fill().Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					for _, text := range b.shown() {
						chip := ui.Box(c).
							Height(20).PaddingX(8).Radius(4).Gap(5).
							AlignItems(ui.Center).
							Label("填入 " + text).
							Tooltip("填入输入框")
						if chip.Hovered() {
							chip.Background(t.SurfaceHover)
						}
						chip.Children(func() {
							ui.Text(c, "❯").FontSize(10).TextColor(t.Accent)
							ui.Text(c, text).FontSize(11).TextColor(t.TextMuted)
						})
						if chip.Clicked() {
							b.draft = text
							b.recall = len(b.sent)
						}
					}
				})
			})
		})
}

// commandBarRow is the composer itself: a prompt glyph, the box, and the button
// that sends. The button names the keystroke, because a text box does not
// produce the enter key for the terminal.
func (w *window) commandBarRow(c *ui.Context) {
	t := c.Theme()
	b := w.commandBarFor()
	tb := w.activeTab()
	col := shellColorsFor(t)

	held := tb != nil && tb.held()
	alive := tb != nil && tb.alive
	sendable := held && alive

	ui.Row(c).
		Height(commandBarHeight).PaddingX(14).Gap(10).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(1, 0, 0, 0).BorderColor(col.hairline).
		Children(func() {
			ui.Text(c, "❯").FontSize(15).TextColor(t.Accent)

			box := ui.TextInput(c, &b.draft).
				Fill().
				Radius(t.Radius)
			box.Background(t.Background)
			if !sendable {
				box.Disabled(true)
				box.Placeholder("该会话当前不可输入")
			} else {
				box.Placeholder("输入命令，回车发送（Shift+回车换行）")
			}
			// Enter sends. The composer takes the key so the terminal does not
			// also see it, and the terminal keeps its caret where it was.
			if box.Submitted() {
				if strings.TrimSpace(b.draft) != "" {
					w.sendCommand(b.draft)
					b.draft = ""
				}
			}
			// History is recalled by the arrows, the way the browser client's
			// is. A multi-line command is edited with them instead, so the
			// arrows only walk history while the draft is one line.
			if !strings.Contains(b.draft, "\n") {
				if box.Shortcut(ui.Shift, ui.KeyUp) {
					b.draft = b.up(b.draft)
				}
				if box.Shortcut(ui.Shift, ui.KeyDown) {
					b.draft = b.down(b.draft)
				}
			}

			send := ui.Button(c, fmt.Sprintf("发送 %s↵", modLabel()))
			if sendable {
				if send.Clicked() {
					if strings.TrimSpace(b.draft) != "" {
						w.sendCommand(b.draft)
						b.draft = ""
					}
				}
			} else {
				send.Disabled(true)
			}
		})
}

// modLabel is the modifier the keyboard's enter is named by. The browser client
// shows ⌘↵ on a Mac and Ctrl↵ elsewhere, and the button here says the same.
func modLabel() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl"
}

// sendCommand puts one command into the shell: it records it in the history,
// sends it with the newline the shell needs, and leaves the box empty.
func (w *window) sendCommand(text string) {
	tb := w.activeTab()
	if tb == nil || !tb.held() || !tb.alive {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	w.commandBarFor().push(text)
	tb.sent = append(tb.sent, text)
	if len(tb.sent) > historyMost {
		tb.sent = tb.sent[len(tb.sent)-historyMost:]
	}
	// The terminal's own caret is where the shell's cursor is, so a command
	// sent from the composer does not move it: the shell echoes what it is given
	// and the cursor follows that.
	//
	// This goes through the same Write the keyboard does, which drops it when
	// the lease is not this window's: the button is disabled in that case, and
	// this is the second guard.
	tb.conn.Write([]byte(text + "\r"))
}
