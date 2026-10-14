package main

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/term-fleet/internal/settings"
)

// systemFamily is what the font list shows for "no family set", which the
// terminal plugin resolves to the system's monospaced font. It reads as a label
// rather than a family name, so it cannot collide with a font someone installs.
const systemFamily = "系统等宽"

// fontChoices are the families the settings window offers. The list is curated
// rather than read off the machine: a family the system lacks falls back to its
// monospaced font on its own, so leaving one out costs nothing.
var fontChoices = []string{
	systemFamily,
	"JetBrains Mono",
	"Maple Mono",
	"SF Mono",
	"Menlo",
	"Monaco",
	"Cascadia Mono",
	"Consolas",
	"Courier New",
	"Noto Sans Mono",
}

const (
	paneTerminal = iota
	paneRelay
	paneServer
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

	pane int

	family  string
	size    float64
	relay   string
	device  string
	secret  string
	ca      string
	relayOn bool
	server  bool

	status string
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
	return s
}

func (s *settingsWindow) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Toolbar(c, func() {
			ui.Text(c, "设置")
			ui.Spacer(c)
			if ui.Button(c, "应用").Clicked() {
				s.commit()
			}
			if ui.Button(c, "恢复默认").Clicked() {
				s.restoreDefaults()
			}
		})
		ui.Tabs(c, &s.pane, "终端", "远程", "服务")
		ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
			switch s.pane {
			case paneTerminal:
				s.terminalPane(c)
			case paneRelay:
				s.relayPane(c)
			default:
				s.serverPane(c)
			}
		})
		if s.status != "" {
			ui.Divider(c)
			ui.Text(c, s.status)
		}
	})
}

func (s *settingsWindow) terminalPane(c *ui.Context) {
	ui.Field(c, "字体", func() {
		ui.Combobox(c, &s.family, fontChoices).OnChange(func() { s.commit() })
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
		ui.TextInput(c, &s.ca).Placeholder("留空则用系统证书库").Fill()
	})
	ui.Field(c, "接入", func() {
		ui.Switch(c, &s.relayOn).OnChange(func() { s.commit() })
	})
	ui.Text(c, "地址形如 wss://relay.example/relay/agent。中继用的是自签或私有 CA 证书时，把 PEM 证书文件路径填进「CA 证书」；改完点「应用」。接入后，登录同一中继的浏览器和手机就能打开这台机器上的会话。")
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
	stored, err := s.store.Update(func(v *settings.Settings) {
		v.FontFamily = familyFromLabel(s.family)
		v.FontSize = s.size
		v.RelayURL = strings.TrimSpace(s.relay)
		v.DeviceID = strings.TrimSpace(s.device)
		v.DeviceSecret = strings.TrimSpace(s.secret)
		v.RelayCA = strings.TrimSpace(s.ca)
		v.RelayOn = s.relayOn
		v.ServerOff = !s.server
	})
	if err != nil {
		s.status = "设置没保存：" + err.Error()
		return
	}
	s.desk.reletter(termFont(stored))
	if err := s.fleet.SetRelay(stored.RelayURL, stored.DeviceID, stored.DeviceSecret, stored.RelayCA, stored.RelayOn); err != nil {
		s.status = "远程渠道没接上：" + err.Error()
		return
	}
	if err := s.fleet.SetServer(!stored.ServerOff); err != nil {
		s.status = "本机服务没起来：" + err.Error()
		return
	}
	s.status = s.summary(stored)
}

func (s *settingsWindow) summary(stored settings.Settings) string {
	parts := []string{"已保存"}
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
	s.relayOn = false
	s.server = true
	s.commit()
}
