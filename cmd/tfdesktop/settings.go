package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/agent"
	"github.com/CarlOpenLab/term-fleet/internal/fonts"
	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// systemFamily is what the font list shows for "no family set", which the
// terminal plugin resolves to the system's monospaced font. It reads as a label
// rather than a family name, so it cannot collide with a font someone installs.
const systemFamily = "系统等宽"

// knownFamilies are suggested on top of whatever is installed on this machine.
// They are a floor for machines whose fonts cannot be read (a bare container,
// say) rather than the whole list: the field takes any family name, because a
// curated list cannot know which builds someone installed. Nerd Font releases
// are named after the family they patch -- "Maple Mono NF" alongside "Maple
// Mono" -- and a prompt drawn with icons needs those glyphs, so the variants
// have to be reachable instead of silently falling back.
var knownFamilies = []string{
	"JetBrains Mono",
	"Maple Mono",
	"Maple Mono NF",
	"SF Mono",
	"Menlo",
	"Monaco",
	"Cascadia Mono",
	"Cascadia Code",
	"Consolas",
	"Courier New",
	"Noto Sans Mono",
	"JetBrainsMono Nerd Font",
	"FiraCode Nerd Font",
	"Hack Nerd Font",
}

const (
	paneTerminal = iota
	paneRelay
	paneServer
	paneUpdates
	paneAppearance
)

// termFont turns stored settings into the terminal plugin's font.
func termFont(s settings.Settings) terminal.Font {
	f := terminal.Font{Family: s.FontFamily}
	if s.FontSize > 0 {
		f.Size = float32(s.FontSize)
	}
	return f
}

// familyFromLabel maps the window's font entry back to what the store keeps.
func familyFromLabel(label string) string {
	if label == systemFamily {
		return ""
	}
	return strings.TrimSpace(label)
}

// labelForFamily is the inverse, for showing a stored family in the window.
func labelForFamily(family string) string {
	if strings.TrimSpace(family) == "" {
		return systemFamily
	}
	return family
}

// settingsWindow is the preferences window: the terminal's font, the relay this
// machine joins, and whether the local service that serves browsers is on.
//
// It edits a draft rather than the store, so a half-typed relay address is
// never written to disk. The discrete controls — the font list, the size, the
// two switches — save as they change; the text fields save when 应用 is
// pressed, which keeps a relay link from reconnecting on every keystroke.
type settingsWindow struct {
	desk  *window
	store *settings.Store
	fleet *fleet

	// win is the window this pane is drawn in. Async work (fetching a
	// certificate, opening the file chooser) finishes off the UI thread and
	// reports back through it.
	win *mygo.Window

	pane int

	// themeSel is the appearance pane's segmented control; it is seeded from the
	// saved preference on every frame, and the control writes a new index here
	// while the frame it was clicked in is still being drawn.
	themeSel int

	family  string
	size    float64
	relay   string
	device  string
	secret  string
	ca      string
	relayOn bool
	server  bool
	// control is whether other clients may take a shell on this machine: the
	// inverse of settings.ControlOff, because the switch reads better as the
	// thing it allows.
	control bool

	// caNote is the one line under the CA field: which certificate is pinned,
	// or why the last attempt failed. It is state, not a transient message,
	// because ui has no other place to show "this is the file you picked".
	caNote string

	// status is the last thing that happened to the draft: saved, or why not.
	status string
	// statusErr marks status as a failure, which the window shows in a
	// different colour from a save.
	statusErr bool
	// savedAt is when status was written. It gives "已保存" a time, so the line
	// reads as an answer to the click that caused it rather than as decoration.
	savedAt time.Time

	// installedMu guards installed, which a background scan fills with the
	// font families on this machine. Until it lands the field suggests
	// knownFamilies only.
	installedMu sync.Mutex
	installed   []string
}

// newSettingsWindow opens on what is on disk right now.
func newSettingsWindow(desk *window, store *settings.Store, f *fleet) *settingsWindow {
	cur := store.Get()
	s := &settingsWindow{
		desk:    desk,
		store:   store,
		fleet:   f,
		family:  labelForFamily(cur.FontFamily),
		size:    cur.FontSize,
		relay:   cur.RelayURL,
		device:  cur.DeviceID,
		secret:  cur.DeviceSecret,
		ca:      cur.RelayCA,
		relayOn: cur.RelayOn,
		server:  !cur.ServerOff,
		control: !cur.ControlOff,
	}
	if s.size <= 0 {
		s.size = settings.DefaultFontSize
	}
	s.caNote = caNoteFor(s.ca)
	s.loadInstalledFamilies()
	return s
}

// loadInstalledFamilies reads the machine's font directories off the UI thread:
// the scan walks every installed font, which is too slow to do while painting.
// The result only feeds suggestions, so arriving late is harmless.
func (s *settingsWindow) loadInstalledFamilies() {
	go func() {
		found := fonts.Families()
		s.installedMu.Lock()
		s.installed = found
		s.installedMu.Unlock()
		s.set(func() {})
	}()
}

// installedFamilies is the scanned list, or nil before the scan finishes.
func (s *settingsWindow) installedFamilies() []string {
	s.installedMu.Lock()
	defer s.installedMu.Unlock()
	return s.installed
}

// familyChoices is what the font field suggests: the system default, the
// families actually installed, then the well-known ones that are not installed
// already. Suggestions only — the field accepts any name.
func (s *settingsWindow) familyChoices() []string {
	choices := []string{systemFamily}
	seen := map[string]bool{systemFamily: true}
	for _, list := range [][]string{s.installedFamilies(), knownFamilies} {
		for _, name := range list {
			if name = strings.TrimSpace(name); name == "" || seen[name] {
				continue
			}
			seen[name] = true
			choices = append(choices, name)
		}
	}
	return choices
}

// loaded is the stored settings with the same fallbacks the fields apply.
// Comparing the draft against this answers "did the user change anything?"
// rather than "does the file spell the defaults the same way as the window?".
func (s *settingsWindow) loaded() settings.Settings {
	cur := s.store.Get()
	if cur.FontSize <= 0 {
		// The terminal reads 0 as its own default; the field shows a number, so
		// a window nobody has touched would otherwise look unsaved.
		cur.FontSize = settings.DefaultFontSize
	}
	return cur
}

// draft is the settings the fields currently describe.
func (s *settingsWindow) draft() settings.Settings {
	cur := s.loaded()
	cur.FontFamily = familyFromLabel(s.family)
	cur.FontSize = s.size
	cur.RelayURL = strings.TrimSpace(s.relay)
	cur.DeviceID = strings.TrimSpace(s.device)
	cur.DeviceSecret = strings.TrimSpace(s.secret)
	cur.RelayCA = strings.TrimSpace(s.ca)
	cur.RelayOn = s.relayOn
	cur.ServerOff = !s.server
	cur.ControlOff = !s.control
	return cur
}

// dirty reports whether the fields hold changes that 应用 has not written yet,
// so the window can say so instead of leaving the user to guess whether the
// click did anything.
func (s *settingsWindow) dirty() bool {
	return s.draft() != s.loaded()
}

// caNoteFor is the one line under the CA field. It reads the file so a wrong,
// missing or expired certificate is visible while the window is open, instead
// of surfacing later as a machine that mysteriously stays offline.
func caNoteFor(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "自签证书的中继必须填这一栏；用公共 CA 签发的中继可以留空。"
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("读不到这个文件：%v", err)
	}
	info, err := agent.DescribeCert(pemBytes)
	if err != nil {
		return fmt.Sprintf("这不是有效的证书文件：%v", err)
	}
	note := fmt.Sprintf("已固定 %s（%s…），%s 到期。", info.Subject, info.Fingerprint[:16], info.NotAfter.Format("2006-01-02"))
	if time.Now().After(info.NotAfter) {
		note += " 这张证书已经过期。"
	}
	return note
}

func (s *settingsWindow) view(c *ui.Context) {
	applyTheme(c, s.store.Get().Theme)
	t := c.Theme()
	// 应用 exists for the fields that commit on it, so it is offered exactly
	// when there is something to apply: a permanently enabled button trains
	// the user to ignore it, and the pending marker is what says otherwise.
	dirty := s.dirty()
	ui.Column(c).Fill().Children(func() {
		ui.Toolbar(c, func() {
			ui.Text(c, "设置")
			ui.Spacer(c)
			if dirty {
				ui.Text(c, "有未保存的改动").FontSize(12).TextColor(t.Warning)
			}
			if ui.Button(c, "恢复默认").Clicked() {
				s.restoreDefaults()
			}
			if dirty && ui.PrimaryButton(c, "应用").Clicked() {
				s.commit()
			}
		})
		paneTabs(c, &s.pane, "终端", "远程", "服务", "更新", "外观")
		// The pane is the whole window, so it scrolls rather than clips: a short
		// window must not hide the 接入 switch under the fold.
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).Fill().Padding(18).Gap(14).Children(func() {
				switch s.pane {
				case paneTerminal:
					s.terminalPane(c)
				case paneRelay:
					s.relayPane(c)
				case paneServer:
					s.serverPane(c)
				case paneAppearance:
					s.appearancePane(c)
				default:
					s.updatesPane(c)
				}
			})
		})
		if s.status != "" {
			ui.Divider(c)
			line := ui.Text(c, s.status).FontSize(11)
			if s.statusErr {
				line.TextColor(t.Danger)
			} else {
				line.TextColor(t.Success)
			}
		}
	})
}

// paneTabs is the strip that chooses a pane, drawn the way the window's session
// tabs are: pills, with the one that is chosen raised. Two idioms for the same
// act — an underline here, a pill there — was the window telling a person that
// its two tab strips were two different things.
//
// TabsBase is what draws it rather than a row of boxes, so the arrows, Home and
// End keep choosing a pane, and the choice keeps following the keyboard.
func paneTabs(c *ui.Context, selected *int, names ...string) {
	t := c.Theme()
	col := shellColorsFor(t)

	tabs := ui.TabsBase(c, selected, len(names))
	tabs.List.
		Height(tabBarHeight).PaddingX(10).Gap(4).
		AlignItems(ui.Center).
		Background(col.pane).
		BorderWidth(0, 0, 1, 0).BorderColor(col.line)
	tabs.List.Children(func() {
		for i, name := range names {
			chosen := i == *selected
			pill := tabs.Tab(i).
				Height(30).PaddingX(13).Gap(7).
				Radius(radiusPill).
				AlignItems(ui.Center).
				Label(name)
			switch {
			case chosen:
				// The same treatment as the shell on screen in the window's own
				// tab strip: the raised face, and the light that falls on it.
				pill.Background(col.tabOn).
					BorderWidth(1).BorderColor(col.line).
					Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.35))
			case pill.Hovered():
				pill.Background(col.field)
			}
			pill.Children(func() {
				label := ui.Text(c, name).FontSize(12.5)
				if chosen {
					label.TextColor(t.Text)
				} else {
					label.TextColor(t.TextMuted)
				}
			})
		}
	})
}

func (s *settingsWindow) terminalPane(c *ui.Context) {
	t := c.Theme()
	ui.Form(c, func() {
		ui.Field(c, "字体", func() {
			// Suggestions only, and no commit on change: this field takes free text,
			// so committing per keystroke would rewrite the settings and rebuild
			// the relay link on every character. Text fields save on 应用.
			ui.Autocomplete(c, &s.family, s.familyChoices()).Fill()
		})
		ui.Field(c, "字号", func() {
			// A stepper stretched across the window reads as a text field; a
			// number is a short thing and gets a short box.
			ui.NumberInput(c, &s.size, 8, 32, 1).Width(110).AlignSelf(ui.Start).
				OnChange(func() { s.commit() })
		})
	})
	ui.Text(c, "字体和字号同时用于这个窗口里已经打开的终端。").FontSize(11).TextColor(t.TextMuted)
}

func (s *settingsWindow) relayPane(c *ui.Context) {
	t := c.Theme()
	ui.Form(c, func() {
		ui.Field(c, "公网地址", func() {
			ui.TextInput(c, &s.relay).Placeholder("wss://relay.example/relay/agent").Fill()
		})
		ui.Field(c, "设备 ID", func() {
			ui.TextInput(c, &s.device).Placeholder("mac-mini").Fill()
		})
		ui.Field(c, "设备密钥", func() {
			ui.TextInput(c, &s.secret).Password().Fill()
		})
		ui.Field(c, "CA 证书", func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.TextInput(c, &s.ca).Placeholder("留空则用系统证书库").Fill()
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, "从服务器获取").Clicked() {
						s.fetchCA()
					}
					if ui.Button(c, "选择文件…").Clicked() {
						s.pickCA()
					}
					if s.ca != "" && ui.Button(c, "清除").Clicked() {
						s.ca = ""
						s.commit()
					}
				})
				ui.Text(c, s.caNote).FontSize(11).TextColor(t.TextMuted)
			})
		})
		ui.Field(c, "接入", func() {
			// AlignSelf(Start): the field's control column stretches, and a
			// stretched switch would sit at the far end of the row, away from
			// the label it belongs to.
			ui.Switch(c, &s.relayOn).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		})
	})
	// The link's own colour: green while it is up, amber while it is not, so
	// the state is readable without parsing the sentence. An unused relay has
	// nothing to report — the agent's state is reset when the link is dropped,
	// and "connecting" about a link nobody asked for would be a lie.
	if s.relayOn {
		if line := s.relayStatusLine(); line != "" {
			color := t.Warning
			if s.fleet != nil && s.fleet.RelayState().Connected {
				color = t.Success
			}
			ui.Text(c, line).FontSize(11).TextColor(color)
		}
	}
	// The client link is a second connection with a credential of its own, so it
	// gets its own line: a fleet with nothing in it and a link that was refused
	// look the same on the 机器 panel, and this is where the difference is read.
	if line := s.reachStatusLine(); line != "" {
		color := t.Warning
		if strings.HasPrefix(line, "●") {
			color = t.Success
		}
		ui.Text(c, line).FontSize(11).TextColor(color)
	}
	ui.Text(c, "地址形如 wss://relay.example/relay/agent。中继用自签或私有 CA 证书时，点「从服务器获取」直接把它取回来并固定，或点「选择文件…」指定已经下载好的 PEM。填完点「应用」。接入后，登录同一中继的浏览器和手机就能打开这台机器上的会话。").
		FontSize(11).TextColor(t.TextMuted)
	ui.Text(c, "接入后，这台机器会拿自己的设备密钥向中继换一枚短时接管令牌（默认 15 分钟），用来自动找到同一中继上的其他机器并接管它们的会话——不需要在这里填任何密码。令牌到期会自动再换一枚。").
		FontSize(11).TextColor(t.TextMuted)
}

// relayStatusLine says what the link is actually doing. Without it, a wrong
// certificate or a rejected credential looks exactly like everything being
// fine, which is how "I filled it in and it is still offline" happens.
func (s *settingsWindow) relayStatusLine() string {
	if s.fleet == nil {
		return ""
	}
	st := s.fleet.RelayState()
	switch {
	case st.Connected:
		return "● " + st.Detail
	case st.Detail != "":
		return "○ " + st.Detail
	default:
		return "○ 正在接入…"
	}
}

// reachStatusLine says whether this window can drive the rest of the fleet: the
// other half of relayStatusLine, which is about this machine joining it. Without
// this line a relay that never minted a credential and a fleet with nothing in it
// look the same on the 机器 panel.
func (s *settingsWindow) reachStatusLine() string {
	if s.fleet == nil || !s.relayOn {
		return ""
	}
	switch {
	case s.fleet.RelayClient() != nil:
		if exp := s.fleet.ControlExpiry(); !exp.IsZero() {
			if left := time.Until(exp).Round(time.Minute); left > 0 {
				return "● 可以接管同一中继上的其他机器（接管令牌还有 " + left.String() + "）"
			}
		}
		return "● 可以接管同一中继上的其他机器"
	case s.fleet.RelayClientErr() != "":
		return "○ 接管链接没接上：" + s.fleet.RelayClientErr()
	default:
		return "○ 正在向中继申请接管令牌…"
	}
}

// fetchCA dials the relay and pins the certificate it presents. It is the same
// trust decision a browser asks a person to make when it warns about a
// self-signed certificate, with two differences: the certificate is shown
// afterwards so it is clear what was trusted, and there is no file to find and
// no path to type.
func (s *settingsWindow) fetchCA() {
	relay := strings.TrimSpace(s.relay)
	if relay == "" {
		s.setNote("先填上面的「公网地址」，再从服务器获取证书。")
		return
	}
	s.setNote("正在从 " + relay + " 获取证书…")
	go func() {
		path, err := fetchAndPinCA(relay, s.store.Dir(), 15*time.Second)
		if err != nil {
			s.setNote("获取失败：" + err.Error())
			return
		}
		s.pinCA(path)
	}()
}

// fetchAndPinCA fetches the relay's certificate and saves it in dir, returning
// the file it wrote. The network and disk work live here so that the button's
// behaviour can be tested without a window to post results to.
func fetchAndPinCA(relay, dir string, timeout time.Duration) (string, error) {
	pemBytes, err := agent.FetchCACert(relay, timeout)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "relay-ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// pinCA adopts path as the CA and saves the settings, so picking a certificate
// takes one click rather than "choose it, then remember to apply".
func (s *settingsWindow) pinCA(path string) {
	s.set(func() {
		s.ca = path
		s.commit()
	})
}

// pickCA lets someone point at a PEM file they already have. The chooser is
// native and blocks until dismissed, so it runs off the UI thread.
func (s *settingsWindow) pickCA() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: s.win,
			Title:  "选择 CA 证书（PEM）",
			Filters: []mygo.FileFilter{
				{Name: "证书文件", Extensions: []string{"pem", "crt", "cer"}},
			},
		})
		if err != nil || len(paths) == 0 || paths[0] == "" {
			return
		}
		s.pinCA(paths[0])
	}()
}

// set runs fn on the UI thread and redraws, so async work can put its result
// on screen.
func (s *settingsWindow) set(fn func()) {
	if s.win == nil {
		fn()
		return
	}
	s.win.Update(fn)
}

// setNote replaces the line under the CA field, without touching the saved
// settings.
func (s *settingsWindow) setNote(note string) {
	s.set(func() { s.caNote = note })
}

func (s *settingsWindow) serverPane(c *ui.Context) {
	t := c.Theme()
	ui.Form(c, func() {
		// The description says what turning this on asks the system for. macOS
		// shows its application-firewall prompt the first time a listening
		// socket appears, and it has no API for an app to pre-authorise itself:
		// the only way through is the user clicking 允许 once, after which it
		// never asks again. A person who meets that prompt cold reads it as the
		// app doing something suspicious, so it is named here, before it happens.
		ui.Field(c, "本机服务", func() {
			ui.Switch(c, &s.server).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		}).Description("开启后本机会开一个只监听 127.0.0.1 的端口。macOS 第一次见到监听中的程序会问一次「是否允许接受传入连接」，点允许即可，之后不再问。")
		// The machine's own shells, and who may type into them. It is here rather
		// than on the remote pane because it is not about the relay: a browser on
		// the LAN answers to it too. It applies on the click, like the switch
		// above, because a switch that guards a machine should not wait for 应用.
		ui.Field(c, "允许被控制", func() {
			ui.Switch(c, &s.control).AlignSelf(ui.Start).OnChange(func() { s.commit() })
		}).Description("关闭后别的客户端仍然能看，但不能接管。")
	})
	if url := s.fleet.BaseURL(); url != "" {
		ui.Text(c, "这台机器上的浏览器可以打开 "+url+"，看到的是窗口里同一批会话。").
			FontSize(11).TextColor(t.TextMuted)
	} else {
		ui.Text(c, "本机服务已关闭：不再为浏览器开端口。窗口照常可用，正在跑的终端也不会停。").
			FontSize(11).TextColor(t.TextMuted)
	}
	// What the switch means, said the way the fleet works: joining a relay is the
	// consent, so this is a way to say no — not a confirmation anyone has to give
	// for each machine that wants in.
	if s.control {
		ui.Text(c, "别的客户端可以接管这台机器的终端：浏览器、手机，或同一中继上的另一台电脑。夺回控制权永远是本机的一键。").
			FontSize(11).TextColor(t.TextMuted)
	} else {
		ui.Text(c, "别的客户端只能只读观看，不能接管、也不能在这台机器上新开终端；本机窗口不受影响。").
			FontSize(11).TextColor(t.Warning)
	}
}

// commit writes the draft to disk and applies it to the running shell: the font
// to the open terminals, the relay link, and the local service.
func (s *settingsWindow) commit() {
	// Read the draft before taking the store's lock: draft() reads the store,
	// and asking for it inside Update would deadlock.
	draft := s.draft()
	stored, err := s.store.Update(func(v *settings.Settings) {
		*v = draft
	})
	if err != nil {
		s.fail("设置没保存：" + err.Error())
		return
	}
	s.caNote = caNoteFor(s.ca)
	s.desk.reletter(termFont(stored))
	if err := s.fleet.SetRelay(stored.RelayURL, stored.DeviceID, stored.DeviceSecret, stored.RelayCA, stored.RelayOn); err != nil {
		s.fail("远程渠道没接上：" + err.Error())
		return
	}
	// The client link is a second connection with a credential of its own: the
	// member one is how this machine joins the fleet, and this one — asked for on
	// the spot, short-lived — is what lets the window drive the other machines'
	// shells. Asking needs the member link to be dialling, so this comes after
	// the relay above.
	if stored.RelayOn && stored.RelayURL != "" {
		if err := s.fleet.Reach(); err != nil {
			s.fail("接管令牌没申请下来：" + err.Error())
			return
		}
	} else if err := s.fleet.SetRelayClient("", "", ""); err != nil {
		s.fail("客户端链接没断开：" + err.Error())
		return
	}
	if err := s.fleet.SetServer(!stored.ServerOff); err != nil {
		s.fail("本机服务没起来：" + err.Error())
		return
	}
	// Who may drive this machine's shells. It is applied to the hub, so a client
	// that is holding a shell right now loses it in the same frame the switch
	// moves, rather than at the next reconnect.
	s.fleet.SetRemoteControl(!stored.ControlOff)
	s.savedAt = time.Now()
	s.statusErr = false
	s.status = s.summary(stored)
}

// fail records that the draft was not applied, so the window can show it in the
// colour that reads as "this did not work" instead of looking like a save.
func (s *settingsWindow) fail(message string) {
	s.statusErr = true
	s.status = message
}

func (s *settingsWindow) summary(stored settings.Settings) string {
	parts := []string{"已保存 " + s.savedAt.Format("15:04:05")}
	if url := s.fleet.BaseURL(); url != "" {
		parts = append(parts, "本机服务 "+url)
	} else {
		parts = append(parts, "本机服务已关闭")
	}
	if stored.RelayOn && stored.RelayURL != "" {
		parts = append(parts, "已接入 "+stored.RelayURL)
	}
	if s.fleet.RelayClient() != nil {
		parts = append(parts, "可接管其他机器")
	}
	return strings.Join(parts, " · ")
}

func (s *settingsWindow) restoreDefaults() {
	s.family = systemFamily
	s.size = settings.DefaultFontSize
	s.relay = ""
	s.device = ""
	s.secret = ""
	s.ca = ""
	s.caNote = caNoteFor("")
	s.relayOn = false
	s.server = true
	s.control = true
	s.commit()
}

// appearancePane is the shell's own look, spelled out as the three modes the
// rail's appearance entry flips through. Naming them here is what makes
// "follow the system" reachable again once the rail has settled on an explicit
// mode; the choice applies as soon as it is made, like the switch on the
// service pane, so there is nothing to save.
func (s *settingsWindow) appearancePane(c *ui.Context) {
	t := c.Theme()
	mode := s.store.Get().Theme
	ui.Form(c, func() {
		ui.Field(c, "界面外观", func() {
			s.themeSel = themeIndex(mode)
			if ui.Segmented(c, &s.themeSel, themeChoices[0].label, themeChoices[1].label, themeChoices[2].label).Changed() {
				s.desk.setTheme(themeChoices[s.themeSel].id)
			}
		})
	})
	ui.Text(c, "当前是「"+themeName(mode)+"」。跟随系统会跟着系统的浅色/深色设置走；另外两项是固定选择，切换立即生效。").
		FontSize(11).TextColor(t.TextMuted)
}
