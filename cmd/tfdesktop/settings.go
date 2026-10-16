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

	family  string
	size    float64
	relay   string
	device  string
	secret  string
	ca      string
	relayOn bool
	server  bool

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
	ui.Column(c).Fill().Children(func() {
		ui.Toolbar(c, func() {
			ui.Text(c, "设置")
			ui.Spacer(c)
			if s.dirty() {
				ui.Text(c, "● 有未保存的改动").TextColor(c.Theme().Warning)
			}
			var apply ui.Element
			if s.dirty() {
				apply = ui.PrimaryButton(c, "应用")
			} else {
				apply = ui.Button(c, "应用")
			}
			if apply.Clicked() {
				s.commit()
			}
			if ui.Button(c, "恢复默认").Clicked() {
				s.restoreDefaults()
			}
		})
		ui.Tabs(c, &s.pane, "终端", "远程", "服务", "更新")
		ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
			switch s.pane {
			case paneTerminal:
				s.terminalPane(c)
			case paneRelay:
				s.relayPane(c)
			case paneServer:
				s.serverPane(c)
			default:
				s.updatesPane(c)
			}
		})
		if s.status != "" {
			ui.Divider(c)
			line := ui.Text(c, s.status)
			if s.statusErr {
				line.TextColor(c.Theme().Danger)
			} else {
				line.TextColor(c.Theme().Success)
			}
		}
	})
}

func (s *settingsWindow) terminalPane(c *ui.Context) {
	ui.Field(c, "字体", func() {
		// Suggestions only, and no commit on change: this field takes free text,
		// so committing per keystroke would rewrite the settings and rebuild
		// the relay link on every character. Text fields save on 应用.
		ui.Autocomplete(c, &s.family, s.familyChoices())
	})
	ui.Field(c, "字号", func() {
		ui.NumberInput(c, &s.size, 8, 32, 1).OnChange(func() { s.commit() })
	})
	ui.Text(c, "字体和字号同时用于这个窗口里已经打开的终端。")
}

func (s *settingsWindow) relayPane(c *ui.Context) {
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
			ui.Text(c, s.caNote)
		})
	})
	ui.Field(c, "接入", func() {
		ui.Switch(c, &s.relayOn).OnChange(func() { s.commit() })
	})
	if s.relayOn {
		ui.Text(c, s.relayStatusLine())
	}
	ui.Text(c, "地址形如 wss://relay.example/relay/agent。中继用自签或私有 CA 证书时，点「从服务器获取」直接把它取回来并固定，或点「选择文件…」指定已经下载好的 PEM。填完点「应用」。接入后，登录同一中继的浏览器和手机就能打开这台机器上的会话。")
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
	ui.Field(c, "本机服务", func() {
		ui.Switch(c, &s.server).OnChange(func() { s.commit() })
	})
	if url := s.fleet.BaseURL(); url != "" {
		ui.Text(c, "这台机器上的浏览器可以打开 "+url+"，看到的是窗口里同一批会话。")
	} else {
		ui.Text(c, "本机服务已关闭：不再为浏览器开端口。窗口照常可用，正在跑的终端也不会停。")
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
	if err := s.fleet.SetServer(!stored.ServerOff); err != nil {
		s.fail("本机服务没起来：" + err.Error())
		return
	}
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
	parts := []string{"已保存 ✓ " + s.savedAt.Format("15:04:05")}
	if url := s.fleet.BaseURL(); url != "" {
		parts = append(parts, "本机服务 "+url)
	} else {
		parts = append(parts, "本机服务已关闭")
	}
	if stored.RelayOn && stored.RelayURL != "" {
		parts = append(parts, "已接入 "+stored.RelayURL)
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
	s.commit()
}
