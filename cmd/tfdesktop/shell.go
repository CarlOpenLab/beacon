package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlOpenLab/term-fleet/internal/protocol"
	"github.com/CarlOpenLab/term-fleet/internal/uitokens"
	"github.com/CarlOpenLab/term-fleet/locales"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The shell is the frame the rest of the window sits in: the band across the
// top (the product's name, the one thing the window has to say about the shell
// on screen, and the controls that act on the window), the strip of open shells
// under it, and the quiet line along the bottom.
//
// It is the browser client's frame in a native window's terms, and the two are
// meant to read as one product: the same band heights, the same strip of pills,
// the same three facts along the bottom in the same order.

// The terminal fills the workspace without a floating card around it.
const paneInset = 0


//go:embed assets/mark.png
var markPNG []byte

// mark is the app's own icon, decoded once. It heads the window's brand and it
// is what stands in the middle of the window while no shell is open.
var mark = decodeMark()

func decodeMark() *ui.Bitmap {
	bmp, err := ui.DecodeBitmap(markPNG)
	if err != nil {
		return nil
	}
	return bmp
}

// markTile draws the mark at a size, or nothing at all when it could not be
// read: a missing picture must not take the window down with it.
func markTile(c *ui.Context, size float32) {
	if mark == nil {
		return
	}
	ui.Image(c, mark).Size(size, size).Radius(size * 0.28).Label("term-fleet")
}

// header is the band across the top of the window: the product's name on the
// left, what the window has to say in the middle, and the controls on the right.
//
// It names the product rather than the shell on screen, which is the browser
// client's arrangement: the tab says which shell this is, the sidebar lists the
// rest, and the one sentence worth the middle of the band — a shell that is not
// this window's to type into — is what the band is actually for.

// headerActions is the right-hand group: what the window can be told to do,
// then the one thing a person does most.
//
// 新建终端 is the accent control, and it is allowed to be: opening a shell is
// what the window is for. The three beside it are quiet icon buttons, and 释放
// only appears while this window is driving a shell that is not this machine's
// — a control that is not about right now has no business being drawn.
//
// 释放控制权 on a shell this machine runs is the one case it is left out of,
// and not because it is rare: releasing it drops the lease and hands it to
// nobody, so the only thing the button does there is stop this terminal
// accepting the typing of the person sitting at it, with no holder to name and
// nothing on screen to say why. A shell of this machine's is typed into here;
// there is no one to give it to.

// headerButtonSize is the square the header's icon buttons take, and the height
// of the primary control beside them. It is the browser client's .btn height.

// headerBanner is the one sentence the window has to say about the shell on
// screen, with the action that answers it. It is the browser client's banner:
// a state in the state's own colour, and one button.

// bannerState is what the band has to say, as a sentence, the word on the
// button that answers it, the colour it is said in, and what the button does.
//
// Only two things are worth the middle of the band, and both are about the
// shell on screen: it has ended, or it is not this window's to type into. What
// just happened to the window — a lease given up, a machine that took a shell
// back — is news rather than state, and the bottom line is where news goes.
func (w *window) bannerState(c *ui.Context) (text, action string, tone ui.Color, run func()) {
	t := c.Theme()
	tb := w.activeTab()
	if tb == nil {
		return "", "", t.TextMuted, nil
	}
	if tb.isEnded() {
		// The window says the state and offers the one thing left to do with it.
		// The sentence does not name the button: a banner that reads
		// "…关闭标签 [关闭标签]" is a banner nobody reads twice.
		return locales.BannerEnded(), locales.TabCloseTab(), t.Danger, func() {
			w.removeTab(tb)
		}
	}
	if tb.isPending() {
		return locales.BannerCreating(), "", t.TextMuted, nil
	}
	if holder := tb.holder(); readOnly(tb) {
		// This machine's own shell is taken back; another machine's was never
		// this window's to begin with, so it is taken over.
		take := locales.ActionTakeBack()
		if tb.machine != machineLocal {
			take = locales.ActionTakeOver()
		}
		if holder == "" {
			// Nobody's, and said as such: a band that named a holder here would
			// have a blank where the name goes. The way in is the same button —
			// a claim on a free lease is honoured at once, with no one to steal
			// it from and no one to tell.
			return locales.BannerReadonlyNobody(), take, t.Warning, func() {
				w.reclaim(tb)
			}
		}
		return locales.BannerReadonlyHolder(holder), take, t.Warning, func() {
			w.reclaim(tb)
		}
	}
	return "", "", t.TextMuted, nil
}

// kbdChip is a keystroke written the way a keyboard writes it: a chip in the
// chrome's quiet greys, or in the accent's own ink when it sits on the accent.

// iconButton is a square control in the chrome: a wash under the pointer, and
// the caption kept as its label so it has a name on screen and in a test.
func iconButton(c *ui.Context, icon *ui.SVG, caption string, size float32) ui.Element {
	t := c.Theme()
	el := ui.Box(c).
		Size(size, size).Radius(radiusSmall).
		AlignItems(ui.Center).Justify(ui.Center).
		Label(caption).Tooltip(caption)
	if el.Hovered() {
		el.Background(t.SurfaceHover)
	}
	el.Children(func() {
		ui.Icon(c, icon).FontSize(size * 0.55).TextColor(shellColorsFor(t).icon)
	})
	return el
}

// fleetColor is the service's colour in the status chip: green while a client
// is attached, amber while the service is listening for one, and quiet while
// there is no service at all.
func (w *window) fleetColor(t *ui.Theme) ui.Color {
	switch {
	case w.fleet == nil:
		return shellColorsFor(t).quiet
	case w.fleet.ClientPresent():
		return t.Success
	case w.fleet.ServerOn():
		return t.Warning
	}
	return shellColorsFor(t).quiet
}

// serviceLine is what the window says about the local service: where it can be
// reached, or that there is no service at all. A window without a service is a
// window, not a crash: it is the state the shell starts in.
func (w *window) serviceLine() string {
	if w.fleet == nil {
		return locales.ServiceOff()
	}
	return w.serviceLabel()
}

// statusHostLine is where the shell on screen runs, and what that machine is:
// its name, its platform, and how many shells it has. The browser client's
// status bar says the same, so a person moving between the two reads one line
// either way.

// statusSessionLine is the shell on screen itself: its id, its role, its size.

// sessionsOn is how many shells a machine is running, as the panel last heard.
func (w *window) sessionsOn(machine string) []protocol.Summary {
	if machine == machineLocal {
		return w.live
	}
	return w.remoteSessions[machine]
}

// body is the workspace: one shell on its card, or the split tree of them.
func (w *window) body(c *ui.Context, pal *uitokens.Palette) {
	if w.split != nil {
		// A zoomed pane is the whole body: the others are still there, one ⌘⇧↩
		// away, which is what kero's zoom means.
		if w.zoomed && w.focus != nil && w.focus.tab != nil {
			w.paneCard(c, pal, w.focus.tab, true, func() { w.focusSplit(w.focus) })
			return
		}
		w.splitView(c, pal, w.split)
		return
	}
	tb := w.activeTab()
	if tb == nil || tb.term == nil {
		// The empty window is one card, as kero's is: what to do about a window
		// with nothing open is a card's worth of chrome, not a paragraph floating
		// on the wash.
		ui.Box(c).Fill().Children(func() {
			card := ui.Column(c).Fill().Radius(uitokens.CardRadius).Clip()
			raised(card.Background(pal.Card), pal)
			card.Children(func() { w.emptyState(c, pal) })
		})
		return
	}
	w.paneCard(c, pal, tb, true, nil)
}

// paneCard is one shell in a card, kero's way: a header with the shell's name,
// where it stands and what it is doing, and the terminal under it, the card's
// corners clipping both. A pane the user is not in fades — its text too — so
// that a split says which of its halves has the keyboard without a border.
func (w *window) paneCard(c *ui.Context, pal *uitokens.Palette, tb *tab, focused bool, focus func()) {
	if tb == nil || tb.term == nil {
		return
	}
	holder := tb.holder()
	multi := w.split != nil && w.split.count() > 1

	ui.Box(c).Fill().Children(func() {
		bg := pal.Card
		if multi && !focused {
			bg = pal.CardDim
		}
		card := ui.Column(c).Fill().MinWidth(0).MinHeight(0).Radius(uitokens.CardRadius).Clip()
		raised(card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 140e6})), pal)
		if focus != nil && card.Pressed() && !focused {
			focus()
		}
		card.Children(func() {
			w.cardHeader(c, pal, tb, focused)
			// The terminal fills the card under the header. It draws its own
			// background, so a full-screen program's color reaches the edges.
			ui.Box(c).Grow(1).MinHeight(0).Children(func() {
				view := terminal.View(c, tb.term).Fill()
				if focused {
					view.AutoFocus()
				}
				// A click on a terminal gives it the keyboard, which is how a
				// person moves between panes: the pane the keyboard is in is the
				// pane on screen.
				if focus != nil && view.Focused() && !focused {
					focus()
				}
			})
			if tb.isEnded() {
				endedBadge(c)
			} else if readOnly(tb) && !tb.isPending() {
				readOnlyBadge(c)
			}
			if multi && !focused {
				// The panes the user is not in fade, text and all. It is drawn at
				// its full strength rather than faded in: the frame that first
				// shows a split is the frame a person reads it in, and a veil
				// still fading would say the wrong pane had the keyboard.
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(pal.Veil)
			}
		})

		// What is left below is the shell that is somebody else's or nobody's,
		// and only ever about the pane on screen.
		if !focused || tb.isEnded() || tb.isPending() || !readOnly(tb) {
			return
		}
		if w.drivenElsewhere(tb) {
			strength := ui.Box(c).Fill().Animate("lease", 1, glowFade)
			leaseGlow(c, float64(strength), c.Preferences().ReduceMotion)
		}
		marker := ui.Box(c).Absolute().Top(uitokens.HeaderH).Start(uitokens.CardGap).Size(2, 2)
		leasePopover(c, marker, &w.leaseOpen, holder, tb.machine != machineLocal, func() {
			w.reclaim(tb)
		})
	})
}

// cardHeader is a card's title row: the state, the shell's name, where it
// stands, the one sentence the window has to say about it, and — while the
// pointer is there — the controls that act on the card.
func (w *window) cardHeader(c *ui.Context, pal *uitokens.Palette, tb *tab, focused bool) {
	bar := c.TitleBar()
	left := float32(uitokens.CardGap + 8)
	if w.sidebarHidden && bar.Left > 0 {
		left = bar.Left + uitokens.CardGap + 8
	}
	h := ui.Row(c).Height(uitokens.HeaderH).Shrink(0).AlignItems(ui.Center).
		Padding(0, 10, 0, left).Gap(8)
	hovered := h.Hovered()
	h.DragWindow()

	name := tb.name()
	h.Children(func() {
		// With the sidebar hidden, the two buttons that live in its top bar come
		// over to the card's header, where kero puts them.
		if w.sidebarHidden {
			if iconButton(c, iconSidebar, locales.ActionToggleSidebar(), 26).Clicked() {
				w.sidebarHidden = false
			}
			if iconButton(c, iconPlus, locales.ActionNewTerminalTip(), 26).Clicked() {
				w.newTab()
			}
		}
		state := w.leaseColor(c.Theme(), tb.machine, tb.id, tb.holder(), tb.alive)
		ui.Box(c).Size(8, 8).Radius(4).Background(state).Shrink(0)
		title := ui.Text(c, name).FontSize(13.5).FontWeight(600).SingleLine().Ellipsis("…").Shrink(1).MinWidth(30)
		if !focused {
			title.TextColor(pal.TextMuted)
		}
		if dir := shortDir(tb.cwd); dir != "" && dir != name {
			ui.Text(c, dir).FontSize(13).TextColor(pal.TextFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0)
		}
		// What it is doing, when that is worth saying: the message a program
		// gave, or how far along it is.
		w.attentionChip(c, pal, tb)
		// What is running in the pane, when it is a program rather than the shell
		// itself: kero's chip, and the answer to "which of these is the agent".
		if sum, ok := w.summaryOf(tb.machine, tb.id); ok && !isShellName(sum.Process) {
			chipBox(c, pal, pal.Chip, pal.TextMuted, func() {
				ui.Text(c, sum.Process).FontSize(12).SingleLine().Ellipsis("…").MaxWidth(160)
			})
		}
		// Where it runs, when that is not this machine: a chip, as kero shows a
		// pane's program.
		if tb.machine != machineLocal {
			chipBox(c, pal, pal.Chip, pal.TextMuted, func() {
				ui.Text(c, w.machineName(tb.machine)).FontSize(12).SingleLine().Ellipsis("…")
			})
		}
		ui.Spacer(c)
		if focused {
			w.cardBanner(c, pal, tb)
			if tb.alive && tb.held() && tb.machine != machineLocal {
				// The lease is state, not a hover: the way to hand a shell back
				// is on screen while the shell is another machine's.
				if chipBox(c, pal, pal.Chip, pal.TextMuted, func() {
					ui.Text(c, locales.ActionRelease()).FontSize(12).SingleLine()
				}).Clicked() {
					w.releaseLease(tb)
				}
			}
			if hovered {
				w.cardActions(c, pal, tb)
			}
		}
	})
}

// cardBanner is the one sentence the window has to say about this card — a
// shell that ended, or one that is not this window's to type into — as a chip
// whose click answers it. It is the old band's banner, in the header's terms.
func (w *window) cardBanner(c *ui.Context, pal *uitokens.Palette, tb *tab) {
	text, action, tone, run := w.bannerState(c)
	if text == "" || w.activeTab() != tb {
		return
	}
	box := chipBox(c, pal, tone.Alpha(0.12), tone, func() {
		ui.Text(c, text).FontSize(12).SingleLine().Ellipsis("…").MaxWidth(280)
		if action != "" {
			ui.Text(c, action).FontSize(12).FontWeight(600).TextColor(tone)
		}
	})
	if action != "" && box.Clicked() && run != nil {
		run()
	}
}

// cardActions is what a card can be told to do, while the pointer is on it:
// split, close the tab, and the lease when the shell is another machine's.
func (w *window) cardActions(c *ui.Context, pal *uitokens.Palette, tb *tab) {
	if iconButton(c, iconSplitRight, locales.ActionSplitRight(), 26).Clicked() {
		w.splitFocused(true)
	}
	if iconButton(c, iconSplitDown, locales.ActionSplitDown(), 26).Clicked() {
		w.splitFocused(false)
	}
	if w.split != nil && w.split.count() > 1 {
		if iconButton(c, iconClose, locales.TabCloseTab(), 26).Clicked() {
			w.closeSplitPane()
		}
	}
}

// shells are the programs a pane runs when it is not running something: a chip
// that said "zsh" beside a shell named "zsh" would be a chip saying nothing.
var shells = map[string]bool{
	"zsh": true, "bash": true, "sh": true, "dash": true, "ksh": true, "fish": true,
	"tcsh": true, "csh": true, "nu": true, "elvish": true, "xonsh": true,
	"pwsh": true, "powershell": true, "cmd": true, "cmd.exe": true,
}

// isShellName reports whether a foreground program is a shell, and so not worth
// a chip of its own.
func isShellName(name string) bool {
	name = strings.TrimSpace(name)
	return name == "" || shells[strings.ToLower(name)]
}

// shortDir is a directory as a person reads it: the home directory as ~, and
// nothing when the shell has not said where it stands.
func shortDir(dir string) string {
	if dir == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if dir == home {
			return "~"
		}
		if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
			return "~" + strings.TrimPrefix(dir, home)
		}
	}
	return filepath.Base(dir)
}

// readOnlyBadge is the mark on a sheet another client is typing into, in the
// corner the browser client puts its own: the word, in the state's colour, on
// the smallest piece of chrome that can hold it.
//
// It is on every shell another client holds, wherever that shell runs. The light
// along the pane's edge is not: it is drawn for this machine's own shells alone,
// because "somebody has this machine's terminal" is a fact about the machine a
// person is sitting at, and a pane that lit up for another machine's shell would
// be lighting up for something its reader has no stake in. The chip is the part
// that reads the same everywhere, so a shell this window only watches still says
// what it is without shouting.
func readOnlyBadge(c *ui.Context) {
	badge(c, locales.StateReadonly(), c.Theme().Warning)
}

// endedBadge is the same mark for a shell that has exited.
func endedBadge(c *ui.Context) {
	badge(c, locales.StateEnded(), c.Theme().Danger)
}

// badge draws a state word in the sheet's top-right corner, as an overlay the
// pointer goes through: it says what the sheet is without taking any of it.
func badge(c *ui.Context, text string, color ui.Color) {
	ui.Box(c).Absolute().Top(12).End(14).PassThrough().Children(func() {
		chip := ui.Row(c).Height(20).PaddingX(8).Radius(radiusSmall).
			AlignItems(ui.Center).Justify(ui.Center).
			Background(color.Alpha(0.12)).BorderWidth(1).BorderColor(color.Alpha(0.42))
		chip.Children(func() {
			ui.Text(c, text).FontSize(10).LetterSpacing(0.4).TextColor(color)
		})
	})
}

// reclaim asks for the lease of a shell another client holds. This machine's own
// shell is claimed outright — the hub honours the machine's window without a
// confirmation, because it is the machine's shell; another machine's is stolen,
// and the holder is told.
func (w *window) reclaim(tb *tab) {
	tb.conn.claiming(tb.machine != machineLocal)
}

// releaseLease gives the lease up without giving up the shell: the process keeps
// running and the tab keeps watching it, this window just stops being the one
// that types into it. The hub hands the lease to the machine the shell runs on,
// so the shell is not left mute.
//
// This is the opposite question to reclaim, and its own button: 释放 is not the
// absence of 接管, it is a thing a person chooses to do — usually so somebody
// else can take the keyboard while this window keeps the output on screen.
func (w *window) releaseLease(tb *tab) {
	// The window let this go a frame ago, so the role frame answering it — which
	// names the machine as the new controller — is not a machine taking the shell
	// back and must not open the popover or tear the link down.
	tb.letGoLease()
	tb.conn.releasing()
	w.setNotice(locales.AppReleasedLease(tb.name()))
}

// emptyState is the window with nothing open. It is not a dead end: it says what
// the window is for and offers the one action that starts it, the way the
// browser client's empty state does.
func (w *window) emptyState(c *ui.Context, pal *uitokens.Palette) {
	ui.Column(c).
		Fill().AlignItems(ui.Center).Justify(ui.Center).Padding(24).
		Children(func() {
			ui.Column(c).Gap(0).MaxWidth(420).AlignItems(ui.Center).Children(func() {
				// kero's empty state: a chip of an icon, the title, one sentence,
				// and the two things a person can do about it.
				ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.Chip).TextColor(pal.TextMuted).
					Margin(0, 0, 18, 0).Children(func() {
					ui.Icon(c, iconTerminal).Size(26, 26)
				})
				ui.Text(c, locales.EmptyTitle()).FontSize(19).FontWeight(650).TextColor(pal.Text)
				ui.Text(c, w.emptyLine()).FontSize(13.5).TextColor(pal.TextMuted).
					MaxWidth(380).TextAlign(ui.Center).LineHeight(1.45).Margin(8, 0, 22, 0)
				ui.Row(c).Gap(10).Children(func() {
					if keroButton(c, pal, "empty-new", locales.ActionNewTerminal(), w.keyFor("session.new"), true) {
						w.newTab()
					}
					if keroButton(c, pal, "empty-settings", locales.ActionSettings(), w.keyFor("settings.open"), false) {
						w.openSettings()
					}
				})
			})
		})
}

// emptyLine is what the empty window says about the machine it would open a
// shell on, so the sentence is true rather than generic.
func (w *window) emptyLine() string {
	machine := w.selectedMachine()
	if w.fleet == nil {
		return locales.EmptyServiceOff()
	}
	if machine != machineLocal {
		return locales.EmptyRemoteNone(w.machineName(machine))
	}
	if n := len(w.sessionEntries()); n > 0 {
		return locales.EmptySidebarSessions(n)
	}
	return locales.EmptyHint()
}

// statusBar is the quiet line along the bottom: the state of the local service,
// where the shell on screen runs, what it is, and what just happened.
//
// The connection is a chip — a dot in a rounded wash — the way the browser
// client states it, so "is anything attached to this machine" reads the same in
// both, and the rest is the same three facts in the same order.

// statusLeaseBar renders the current shell's input lease state (只读 / 控制中)
// and corresponding action buttons (接管 / 释放控制权) at the bottom right.

// buildLine is the version and the platform, the way a status bar says it.
