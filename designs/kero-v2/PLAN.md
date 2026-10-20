# term-fleet × Kero v2：风格移植与特性移植计划

> 目标：把 egoist 的 **Kero v2**（macOS 原生终端工作台，Go + Mygo）的视觉语言
> **一比一**落到 term-fleet 的桌面端与浏览器端，把它真正好用的特性拿过来，
> 同时**不丢**term-fleet 自己的核心：多机器舰队、中继、单控制者租约、字节偏移续接。
>
> 前提（已确认）：桌面 + 浏览器两端都改；两端都去掉标签栏；允许改 daemon 协议；
> 照搬 Kero 的浅/深双主题，**浅色为默认**。
>
> 本文是实施计划，不是设计稿。视觉事实全部来自 Kero 源码（`theme.go`、
> `elevation.go`、`view.go`、`panes.go`、`controls.go`、`website/components/AppWindow.tsx`
> 与 `website/styles/global.css`），数值可直接抄。

---

## 0. 不变量（任何改动都不得破坏）

| 不变量 | 现状证据 | 本计划如何保住 |
| --- | --- | --- |
| PTY 归 daemon（`tfd`）所有，桌面窗口只是普通客户端 | `internal/session`、`cmd/tfdesktop/link.go` | 不动进程/PTY 归属；桌面仍经本地 hub 的 lease 走 |
| 单控制者 + 旁观者，租约带 epoch 与过期 | `internal/hub/hub.go`、`docs/takeover.md` | 只换皮，不动 `claim/release`；新增状态字段不动租约 |
| 字节偏移历史与断线续接 | `protocol.Attach{cursor}`、`ring.go` | 换皮与加字段都不碰游标语义 |
| 中继 / 隧道 / 控制令牌 | `internal/relay`、`internal/tunnel` | 不动；机器分组只是换个画法 |
| 桌面 + 浏览器 + 手机同一份会话 | 同一 WS 协议 | 状态字段两端同源，避免各算一套 |
| 关闭 ≠ 杀进程；结束 ≠ 关闭 | `app.go` `kill` / `removeTab` | 词汇与二次确认保留（只改样式） |
| 机器自己的窗口永不被 `controlOff` 拒绝 | `hub` | 不动 |

---

## 1. Kero 拆解

### 1.1 视觉规范（照抄）

**尺寸**（`view.go:20-26`、`panes.go:15`）

| 名称 | 值 | 用处 |
| --- | --- | --- |
| `barMid` | 30 | 标题栏内容的中线（红绿灯、侧栏按钮、卡片头部都对齐它） |
| `topBar` | 60 | 侧栏顶部带 |
| `cardGap` | 8 | 卡片四周留白、分屏之间的缝 |
| `headerH` | 44 | 卡片头部（= 2×(30−8)） |
| `rowH` | 34 | 会话行 |
| 侧栏宽 | 268（200–440，双击复位） | |
| 卡片圆角 | 14 | 行圆角 8，控件 6–8，chips 药丸 |
| 侧栏组头 | 28 | 计数用 tabular numerals |
| 侧栏脚 | 36 | 三个图标按钮 + 右侧状态 chips |

**颜色**（`theme.go` `lightPalette` / `darkPalette`，直接取值）

| token | 浅色 | 深色 |
| --- | --- | --- |
| sidebar | `#eceef4` | `#141418` |
| text / muted / faint | `#1d1d1f` / `#6e6e78` / `#a3a3ad` | `#ececf0` / `#a9a9b4` / `#83838f` |
| rowHover / rowSelected | `rgba(0,0,0,.04)` / `#ffffff` | `rgba(255,255,255,.05)` / `rgba(255,255,255,.10)` |
| card / cardBorder | `#ffffff` / `rgba(0,0,0,.07)` | `#232328` / `rgba(255,255,255,.08)` |
| cardDim / veil | `rgba(255,255,255,.58)` / `rgba(250,248,252,.28)` | `rgba(35,35,40,.5)` / `rgba(14,13,18,.42)` |
| control / controlHover | `#ffffff` / `#f7f7f9` | `#404047` / `#4a4a52` |
| chip / divider | `rgba(0,0,0,.045)` / `rgba(0,0,0,.06)` | `rgba(255,255,255,.07)` / `rgba(255,255,255,.07)` |
| attention / attnSoft | `#c27b0f` / `rgba(232,154,40,.13)` | `#f0a33a` / `rgba(240,163,58,.15)` |
| done / onDone | `#1d1d1f` / `#ffffff` | `#ececf0` / `#161618` |
| danger / accent | `#d93a35` / `#3b6cf6` | `#ff6961` / `#6b93ff` |
| kbd / kbdBorder | `rgba(0,0,0,.04)` / `rgba(0,0,0,.09)` | `rgba(255,255,255,.06)` / `rgba(255,255,255,.1)` |

**wash（窗口背景）**（`theme.go:196-256`）：三到四段 Oklab 线性渐变（180°）+
225° 辉光，五种：`rex`（粉紫，默认）/ `mist` / `meadow` / `dusk` / `plain`；
开着 vibrancy 时底色 alpha 0.95。

**终端主题**（`theme.go:158-186`）：浅色前景 `#1d1d1f`、背景 = card、光标同前景、
选区 `#cfdcfb`，16 色是 Apple 系统色的加深版；深色前景 `#e6e6ea`、背景 `#232328`、
选区 `#2f4170`。**两套都抄。**

**层次（elevation）**（`elevation.go`）

- `raised`：浅色 = 两层极轻阴影；深色 = 1px 受光边（text 18%→6% 竖向渐变）+ 三层暗影。
- `chosen`（选中行）：浅色 = 一圈 0.5px 暗环 + 0.5px 贴地阴影；深色 = `raised`。
- `menu`：浅色 = `#fbfbfb→#f4f4f5` 竖向渐变 + 内外白边 + 上 0.5px 黑 30% + 5/10 阴影。
- `floating`（面板/对话框）：`raised` + `0 16 40` 大阴影。
- `button`：高 30，圆角 8，`control` 面；主按钮 accent 面白字；按下混 8% 黑。
- `field`：高 26，圆角 7，内嵌 1px 边；聚焦时 accent 外发光 + accent 内描边。

**字体**：随包 JetBrains Mono Nerd Font（OFL），终端默认它；UI 用系统 UI 字体。
字号：会话名 13.5，元信息 12.5，卡片标题 13.5/单 pane 14，设置行 13.5，组标题 12，
kbd 11，品牌 14。

**动效**：侧栏 260ms `cubic-bezier(.25,1,.5,1)` 且按整像素量化；行底色 110ms；
未聚焦 veil 160ms；卡片 hover 控件 120ms 淡入；拖拽落点弹簧 300ms/bounce .12，
落位 420ms/bounce .18；状态图标本身就是动画（working 转弧、waiting 脉动点）。

### 1.2 特性分级

| 级别 | 特性 | 来源 | 我们为什么值得要 |
| --- | --- | --- | --- |
| **P0** | 状态管线：`working / waiting / idle / error / exited` + 未读 + 消息 + 进度 | `detect.go`、`osc.go`、`session.go` | 这是 Kero 的灵魂：舰队里哪台机器的哪个 agent 在等你，一眼看到 |
| **P0** | 状态图标五态 + 微动画 | `view.go:567-656` | 便宜的辨识度，直接决定"像不像 Kero" |
| **P0** | 侧栏 = 分组会话 + 计数 + "N need you" + footer 计数 chips + 连接点 | `view.go:247-542` | 取代我们现在的双面板 + 状态栏，信息更少但更准 |
| **P0** | ⌘J 跳到需要你的会话（含展开折叠组、无则提示） | `app.go:1007` | agent 工作流的核心按键 |
| **P0** | 卡片式 pane：头部（状态/标题/目录/进程 chip/悬停控件）+ 8px 缝 + 14 圆角 | `panes.go:333-423` | 视觉主体 |
| **P0** | 未聚焦 veil + `cardDim` | `panes.go:161-202` | 分屏时"我在哪"一眼清楚 |
| **P0** | 命令面板：会话 + 机器 + 命令 + 设置页，模糊打分 | `palette.go` | 会话一多，侧栏不够用；手机/浏览器同样受益 |
| **P0** | 设置窗口：分组面板 + 新控件（弹出钮/搜索钮/分段/开关/滑杆）+ 实时终端预览 | `settings_view.go`、`controls.go` | 现在的 4 个 tab 撑不住新增项 |
| **P0** | 双主题 + wash + 终端主题 + 外观/背景设置 | `theme.go` | 用户明确要"一模一样"，浅色是 Kero 的主场 |
| **P0** | 随包等宽字体 + 字号/行高/字重/连字 | `fonts.go` | 观感的另一半；现在只能挑系统字体 |
| P1 | 快捷键可重绑 + actions 注册表 + 菜单/面板/设置同源 | `internal/actions` | 新增十几个动作后，硬编码必失控 |
| P1 | 指针处 tooltip（目录、进程、消息、整行故事） | `view.go:167-211` | macOS 原生手感，且能塞下我们更多的元信息 |
| P1 | 侧栏宽度拖拽、整像素动画、折叠组旋转箭头、⌘1–9、Ctrl+Tab | `view.go:132-148,375-401` | 手感 |
| P1 | 分屏 zoom（⌘⇧↩）、divider 抓手、双击回中 | `panes.go:114-153` | 我们的分屏现在只有拖分隔线 |
| P1 | 相对时间 `now/4m/2h/3d`（20s 心跳） | `proto.Ago` | 行上比绝对时间有用 |
| P1 | 空状态 + 可复制的 agent CLI 片段 | `view.go:711-733` | 教会用户怎么用（也预告 P2 的 CLI） |
| P2 | `tf` CLI：`ls / new / send / wait / status / report / focus` | `internal/cli` | 让 agent 驱动舰队；`report` 走 OSC 7501 |
| P2 | agent skill/hook 安装（Claude/Codex/OpenCode） | `internal/cli/skill.go` | 状态自动上报与自命名会话 |
| P2 | 会话在窗口退出后继续（daemon 常驻） | `README` | Kero 的招牌；但与我方"PTY 是窗口子进程"的现状冲突，需单独决策 |
| P2 | 屏幕抓取式 agent 识别（herdr manifests） | `detect.go` | 需 daemon 里有 VT，见 §7 阻塞项 |
| P2 | 配置文件（Ghostty 语法、保留注释、热加载） | `settings/config.go` | 好用，但 JSON 现状不痛 |
| X | libghostty 渲染 diff 协议、Ghostty 主题库、Dock 弹跳、远程主机计划 | — | 要么已有等价物（中继），要么与 xterm 客户端冲突 |

---

## 2. 目标形态

### 2.1 桌面（tfdesktop）

```
┌──────────────┬──────────────────────────────────────────────────────────┐
│ ●●●   [▤][+] │  ⬤ snake · claude     ~/dev/snake    [waiting] [claude]  │
│              │                                          [⇔][⇕][⤢][×]   │
│ snake    2   │ ┌──────────────────────────────────────────────────────┐ │
│  1 need you  │ │                                                      │ │
│  ● Review…   │ │  终端（焦点 pane，满卡片，无内边距）                  │ │
│  ◌ Add high… │ │                                                      │ │
│              │ └──────────────────────────────────────────────────────┘ │
│ website  3   ├──────────────────────────────────────────────────────────┤
│  ◔ Fix e2e   │  ⬤ lazygit            ~/dev/snake              [lazygit] │
│  ✓ Docs      │ ┌──────────────────────────────────────────────────────┐ │
│              │ │  未聚焦 pane：cardDim + veil（连文字一起淡）          │ │
│              │ └──────────────────────────────────────────────────────┘ │
│ [▤][⌕][⚙]    │                                                          │
│        ◌1 ◔1 ✓1                                                       │
└──────────────┴──────────────────────────────────────────────────────────┘
   wash 渐变背景（rex/mist/meadow/dusk/plain）
```

- 去掉顶部 44px header 与底部 28px 状态栏：**它们的信息全部搬进侧栏脚 + 卡片头**。
- 侧栏分组 = **机器**（本机在第一组，标题不显示"机器"而是主机名；中继上的机器用
  roster 名），组头右侧是会话数 + "N need you"；这一层就是把 Kero 的 project 换成 machine，
  其余（折叠、计数、悬停 +）照抄。
- 会话行就是 Kero 那一行：状态图标 · 名字 · 相对时间 · （按住 ⌘ 变 ⌘1–9 提示）。
- 分屏保留现有 `splitNode`（叶子是会话，不是 pane），只换画法：缝隙 8px、
  divider 悬停出抓手、双击回 50%。
- 机器面板不再单独存在：机器状态（在线/离线/中继/只读）折进组头与 tooltip；
  "添加机器/刷新"进组头右键菜单与侧栏脚。

### 2.2 浏览器 / 手机（web）

```
┌──────────────┬───────────────────────────────────────────┐
│ 会话 12   ↻   │  ⬤ zsh                     ~/dev/term-fleet │
│              │ ┌─────────────────────────────────────────┐ │
│ carl-mini 3  │ │                                         │ │
│  ● zsh       │ │  xterm（卡片，圆角 14）                  │ │
│  ◌ claude    │ │                                         │ │
│              │ └─────────────────────────────────────────┘ │
│ mac-studio 2 │ ┌─────────────────────────────────────────┐ │
│  ◔ dev       │ │  composer（底部浮起的一条，保留）         │ │
│              │ └─────────────────────────────────────────┘ │
│ [⌕] [⚙]      │  keys 触屏行（保留）                        │
│        ◌1 ◔1 │                                             │
└──────────────┴───────────────────────────────────────────┘
```

- 标签栏删除，侧栏就是会话列表；手机上侧栏是抽屉（现有 `☰` 保留）。
- 一次只显示一个会话：切换即 attach（沿用 `Attach{cursor}` 续接），离开即 detach；
  同一时刻最多保留 N 个 xterm 实例（LRU，Kero 的 12 个上限）。
- composer / 触屏按键 / 只读横幅 / 接管弹窗**全部保留**，只换皮。
- 手机端：卡片铺满、圆角降为 0（贴边），composer 贴底加 safe-area。

---

## 3. 技术方案

### 3.1 Token 单一来源（先做，后面所有东西都靠它）

新建 `internal/uitokens`（Go 包，无 UI 依赖）：

```go
type Palette struct{ Sidebar, Text, TextMuted, TextFaint, RowHover, ... ui.Color }
var Light, Dark Palette
var Washes = map[string]Wash{...}
var TermLight, TermDark = terminal.Theme{...}, terminal.Theme{...}
const BarMid, CardGap, HeaderH, RowH, TopBar, ... = ...
```

- 桌面直接 `uitokens.Light`。
- web 用 `go generate` 产出 `web/src/tokens.css`（`:root` + `[data-theme="dark"]`），
  由 `internal/uitokens/gen` 写文件；`tokens_test.go` 重新生成并与仓库文件比对，
  不一致就 fail —— 两端永远同一个数，结束现在 `palette.go` 与 `style.css` 各写一份、
  注释里还写着"token for token"却已经漂移的状态。
- `cmd/tfdesktop/palette.go` 的 `shellColors` 与 `theme.go` 的 `applyTheme` 被这套取代；
  `lightTheme` / `themeChoices` / `flipTheme` 这些死代码删除或复活（复活：设置里真的能选）。

### 3.2 桌面壳体重构（文件级）

| 文件 | 动作 |
| --- | --- |
| `cmd/tfdesktop/theme.go` | 重写：`paletteFor(c)` + `borderless(c)` + `backdrop(c, pal, row)`；`applyTheme` 从"强制深色"改为跟随设置+系统 |
| `cmd/tfdesktop/palette.go` | 保留 `chromeMetrics` 但取值换成 Kero 的；`shellColors` 删除 |
| `cmd/tfdesktop/elevation.go`（新） | 移植 `raised / chosen / menu / floating / darkShadow / litEdge / face / button / toggle` |
| `cmd/tfdesktop/controls.go`（新） | 移植 `iconButton / chip / kbd / popup / searchPopup / menuSpot / menuPanel / menuRow / field / textField / slider / segmented` |
| `cmd/tfdesktop/status.go`（新） | 移植 `statusIconP`（五态 + 进度 + 动画）、`ago` |
| `cmd/tfdesktop/palette_panel.go`（新） | 命令面板（会话/机器/动作/设置页 + 模糊打分） |
| `cmd/tfdesktop/nav.go` | 重写侧栏：分组头、会话行、footer chips、宽度拖拽、整像素动画；机器面板删除 |
| `cmd/tfdesktop/shell.go` | header/statusBar 删除，改成 `cardHeader` + `paneCard`；保留 banner/lease popover 的语义 |
| `cmd/tfdesktop/split.go` | `splitView` 改成 Kero 的 gap + divider 抓手 + 双击回中；加 zoom |
| `cmd/tfdesktop/app.go` | 帧重排成 `Row[sidebar | Column[cards]]` + wash + `DialogBase` 弹层；快捷键改走 actions |
| `cmd/tfdesktop/actions.go`（新） | 动作注册表（id/标题/菜单/默认键/是否可远程），取代散落的 `c.Shortcut` |
| `cmd/tfdesktop/settings.go` | 重写为 Kero 的窗口形态：左侧页列表 + 分组面板 + 跨页搜索 + 快捷键录制 |
| `cmd/tfdesktop/main.go` | 窗口加 `Vibrancy: true`、`TitleBarHidden`（已有）、`BackgroundColor` 跟随 wash；`mygo.Theme.SetSource` 不再钉死 |

### 3.3 状态管线（daemon 侧，P0 的地基）

新建 `internal/status`：

1. `osc.go`：移植 Kero 的扫描器 —— OSC 7501（`state=working/idle/done/blocked/error/clear`、
   `kind=permission|question|auth`、`progress`、`msg`/`title` base64、多任务 `id`）、
   OSC 133（`;A` 结束实时上报、`;D;<code>` 记录退出码）、OSC 0/2 标题（spinner 判定）、
   OSC 9;4 进度、bell、OSC 9 notify。**纯字节扫描，不需要 VT。**
2. `proc_unix.go` / `proc_windows.go`：前台进程名（macOS/Linux：`tcgetpgrp` 或 `KERN_PROC`；
   Windows：`GetConsoleProcessList` 不可用时退化为子进程名）。term-fleet 已有取 cwd 的
   平台代码，可复用同一套调用时机。
3. `session.go`：状态聚合（waiting 5 › error 4 › exited+unread 3 › unread 2 › working 1，
   同分取焦点 pane）、未读（状态变化时不在屏上 ⇒ unread；`view` 或输入清除）、
   `working/blocked` 随前台进程消失而结束、`done/error` 留到用户输入、
   idle 反推需稳 700ms 才离开 working（防闪烁）。
4. `hub` 侧：每帧/每 250ms 汇总进 `state` 广播。

**协议**（`internal/protocol`，版本 +1，旧客户端显式提示"服务端已升级"）：

```go
type Summary struct {
    ID, Title string
    Machine   string
    Dir       string
    Cols, Rows int
    Controller string   // 已有
    Alive     bool      // 已有
    // 新增
    Status    string    // idle|working|waiting|error|exited
    Unread    bool      //
    Message   string    // 需要你/进度/失败原因，≤2KB
    Kind      string    // permission|question|auth
    Progress  int       // -1 无
    Process   string    // 前台程序名
    Since     int64     // 状态开始时间，毫秒
}
```

- 浏览器端：侧栏状态位 + `document.title` 上的计数 + `Notification`。
- 桌面端：状态位 + 原生通知 + Dock 徽标（`mygo.App.SetBadgeCount`，若不可用则跳过）
  + ⌘J。

### 3.4 桌面字体

- 把 `resources/fonts/JetBrainsMonoNerdFont-*.ttf`（OFL）打进包，启动时 `ui.RegisterFont`；
  `resources/fonts/OFL.txt` 一起带上。
- 设置里字体选择器 = Kero 的 `fontChoices` + `internal/fonts` 扫描到的已安装字体，
  选中即 `reletter` 现有终端（已有机制）。
- 字号/行高/字重/连字写进 `terminal.Font`：mygo 0.5.5 的 `terminal.Font` 已有
  `Size / Weight / Features / LineHeight / Thicken` 全部字段，`ui.RegisterFont` 也在
  （`ui/window.go:37`），**不需要升级 mygo**。

### 3.5 web 重构

- `web/src/style.css` 的 `:root` 换成生成的 `tokens.css`；现有语义 token 名保留，
  值改为 Kero 的（`--bg` → wash 之上的 card 色，`--fg` → text…）。
- `app.ts`：删 `tabStrip` 与 `tabs` 的 DOM 语义，改成 `activeSession` + 单 pane 容器；
  会话列表点击 → `attach`；保留 `histBar`/`composer`/`keyHost`/`inspect`；
  `tabs` 的数据结构可以保留（内部仍以 bundle 缓存 xterm），只是不再画标签栏。
- 移动端：`fleet.css` 的 phone 段跟着改（卡片贴边、抽屉、safe-area）。
- 状态位、footer chips、⌘K 面板（桌面宽度下）同步。

---

## 4. 里程碑

> 每个里程碑都以**可看的截图**收尾（`TF_RENDER` 预览 + 浏览器 CDP 截图），
> 并保持 `go test ./...` 与 `pnpm run check` 绿。

### M1 · Token 地基（1–2 人日）

- `internal/uitokens` + `go generate` → `web/src/tokens.css` + parity test。
- 桌面：`theme.go`/`elevation.go` 就位，`applyTheme` 真的跟随浅/深；
  窗口开 vibrancy，背景换成 wash。**此时界面结构不变，只换颜色与层次。**
- 验收：浅/深两张截图；`tokens_test.go` 通过；`nav_test.go:479` 的像素探针按新值更新。

### M2 · 桌面侧栏 + 卡片（3–4 人日）

- `nav.go` 重写（分组 = 机器、状态图标、footer chips、宽度拖拽、折叠动画、⌘1–9）。
- `shell.go`：header/statusBar 删除，`cardHeader` + `paneCard` + 未聚焦 veil。
- `split.go`：gap 8、divider 抓手、双击回中、zoom。
- 验收：单 pane / 三分屏 / 折叠 / 侧栏收起 四张截图；`nav_test.go:50-89` 的侧栏
  断言、`shell_test.go` 的 header/status 断言改写；`lease_test.go` 的 glow 探针
  （`paneInset±2`）随卡片留白调整。

### M3 · web 换肤 + 去标签栏（3–4 人日）

- tokens 接入、卡片化、侧栏分组、状态位、抽屉/手机段。
- 删标签栏后修 attach/detach 生命周期与 LRU（最多 12 个 xterm）。
- 验收：`pnpm run acceptance` + `acceptance:relay-browser` 全绿；手机宽度截图。

### M4 · 控件 + 设置窗口（3–4 人日）

- `controls.go` 全套 + 设置窗口重写：页列表（通用/外观/终端/中继/服务/通知/键盘/高级）、
  分组面板、跨页搜索、快捷键录制、终端实时预览、背景缩略图。
- 字体打包 + 选择器。
- 验收：设置各页截图；`settings_test.go` 按新标签重写；语言切换（zh/en）不破。

### M5 · 状态管线（4–5 人日）

- `internal/status` + hub 广播 + 协议字段 + 两端状态位 + 通知 + ⌘J + Dock 徽标。
- 验收：单测（OSC 7501 各类畸形/多任务/清理、133、spinner、未读、优先级聚合）；
  脚本化 e2e：`printf '\e]7501;state=blocked...\a'` 让浏览器与桌面同时变 waiting。

### M6 · 交互打磨（3–4 人日）

- 命令面板、⌘J、tooltip、相对时间、空状态、右键菜单带快捷键、toast、确认框。
- 验收：面板/菜单/tooltip 截图；键盘全流程手测清单。

### M7 · 收尾（1–2 人日）

- README / CHANGELOG / `docs/` 截图更新；`PLAN.md` 里程碑补记；
  发布前 `go vet ./... && go test ./... && pnpm run check && pnpm run acceptance:all`。

合计约 **18–25 人日**（不含 §6 的 P2）。

---

## 5. 需要改的测试（提前认领，避免"改不完"）

| 测试 | 现在钉住什么 | 改法 |
| --- | --- | --- |
| `nav_test.go:479-489` | 像素 `RGBAAt(899,300)` 是深灰、且没有主题开关 | 改成按当前 palette 断言；主题开关的断言反向 |
| `nav_test.go:50-89` | 侧栏与内容之间的缝 | 新布局：卡片留白 8，缝消失 |
| `lease_test.go:107-156` | 沿 pane 边缘扫 glow（`paneInset±2`） | 卡片有 8px 留白，探针按新几何重算 |
| `shell_test.go:85-99` | `#111111` 背景、`#d0d0d0` accent、光标色 | 改成 `uitokens.Light/Dark` 取值 |
| `shell_test.go:17-35` | header/status 文案、inspector 不可见 | header/status 不存在了，断言改为侧栏脚与卡片头 |
| `render_test.go` | 预览图清单 | 增加浅/深 × wash × 分屏 × 面板 × 设置页 |
| `i18n_test.go` | en 与 zh 键对齐 | 新增文案两处都加（用 `go tool mygo generate`） |
| `scripts/acceptance*.mjs` | DOM 类名（`.tf-tabs` 等） | 删标签栏相关断言，加状态位断言 |

---

## 6. 留给下一版的（P2，需单独开计划）

1. `tf` CLI + `tf report`（OSC 7501）+ agent skill/hook 安装 —— 让 agent 驱动舰队，
   这是 Kero 对 agent 友好的关键，但要新起一个 `cmd/tf` 与协议入口。
2. 会话在窗口退出后继续（daemon 常驻）：现在 `tfdesktop` 内嵌 hub，PTY 是窗口子进程，
   退出即结束。要做成 Kero 那样，得让桌面连接一个常驻 `tfd`（用户级服务），
   并持久化 `state.json` + 每个会话的历史。**架构级改动，单独评审。**
3. 屏幕抓取式 agent 识别（herdr manifests）：需要 daemon 内有 VT 影子终端。
   阻塞项见 §7。
4. 配置文件（Ghostty 语法、保留注释、热加载）与 Ghostty 主题库。

---

## 7. 风险与阻塞

| 风险 | 说明 | 缓解 |
| --- | --- | --- |
| **"一模一样"的边界** | Kero 的质感有一半来自 macOS 原生字体渲染、vibrancy 与 Traffic Lights；Windows/Linux 只能近似 | 承认差异：macOS 走 vibrancy，其它平台用 wash 平面；红绿灯位置用 `c.TitleBar()` 适配 |
| **桌面没有 VT** | 状态管线里的"屏幕抓取"不可做（mygo 的 `vt` 是 internal 包，daemon 无法 import） | P0 只做 OSC + 进程名 + 标题 spinner（Kero 规则里最稳的那部分）；屏幕抓取留 P2，届时向上游要 `vt` 导出或用 `terminal.Terminal{Conn}` 影子终端 |
| **协议 +1** | 两端必须同时升级；中继上可能还有旧客户端 | 版本号 + 明确文案；relay 只转发，不受影响 |
| **web 去标签栏** | 是行为改变（多会话不能同屏），手机端要重做导航 | 侧栏 + ⌘K 面板补足；保留"最近会话"顺序 |
| **像素级测试** | 4 处测试直接钉颜色/几何，换肤必挂 | 已在 §5 列出，M1/M2 内就改，不拖到最后 |
| **mygo 版本** | 已核对 0.5.5：`Context.TitleBar` / `Vibrancy`、`Element.InsetShadow` / `AnimateColor` / `AnimatePose` / `Pose` / `Clip` / `PassThrough` / `PointerPosition` / `DragWindow` / `ScrollIntoView` / `Drag` / `Drop` / `DialogBase` / `Overlay` / `TooltipBase`、`ui.RegisterFont`、`terminal.Font{Size,Weight,Features,LineHeight,Thicken}` **全部存在**，无需升级 | 若实现中发现 Kero 用到的 0.5.6-dev 专属行为，再评估升级；升级时先跑 M1 的 token 与三端测试 |
| **嵌入式 web** | `go:embed` 需要先 `pnpm --dir web run build` | M3 起把 `build:web` 放进 `make check` 的本地流程 |

---

## 8. 开工顺序（建议）

```
M1 tokens ─┬─> M2 桌面壳 ─┬─> M4 控件/设置 ──> M6 打磨 ─> M7 收尾
           └─> M3 web 壳  ┘
M5 状态管线（可并行，依赖 M1 的 token 与 M2/M3 的行组件）
```

先做 M1（纯色板与层次，风险最低、观感提升最大、能立刻出图对比），
M2/M3 并行不了（同一个人），所以按 M2 → M3 推进；M5 在 M2 之后插进来，
因为它要改侧栏行与卡片头。

---

## 9. 实施状态（0.5.0）

### 已完成

| 里程碑 | 内容 | 落点 |
| --- | --- | --- |
| M1 | 颜色 / wash / 终端配色 / 尺寸单一来源，浏览器 token 由它生成并有防漂移测试 | `internal/uitokens/tokens.go`、`gen/main.go`、`tokens_test.go`、`web/src/tokens.css` |
| M1 | 双主题真正接通（跟随系统 / 白天 / 暗黑），窗口开 vibrancy，wash 上浮卡片 | `cmd/tfdesktop/theme.go`、`palette.go`、`main.go` |
| M2 | 侧栏改为机器分组：组头（状态点、计数、折叠箭头、悬停 +）、34px 会执行、相对时间、脚部图标与计数 chips、宽度拖拽 | `cmd/tfdesktop/sidebar.go`、`nav.go` |
| M2 | 卡片：44px 头部（状态、名字、目录、机器/进程 chip、消息 chip、悬停控件）、8px 缝、14 圆角、未聚焦 veil、空态也在卡片里 | `cmd/tfdesktop/shell.go`、`split.go` |
| M2 | 旧的顶栏、状态栏、会话面板、机器面板删除（约 400 行死代码） | `shell.go`、`nav.go` |
| M5 | daemon 状态管线：OSC 7501、OSC 9、响铃、标题转圈、输出活跃度；`idle/working/waiting/error/exited` + 未读 + 消息 + 类型 + 进度；协议 v2；hub 状态变更广播 | `internal/status`、`internal/session`、`internal/hub`、`internal/protocol` |
| M5 | 五态状态图标（转弧 / 进度弧 / 脉冲环 / 红盘 / X 环 / 勾）、⌘J、脚部计数跳转、窗口标题计数、未读加粗 | `cmd/tfdesktop/status_glyph.go`、`attention.go` |
| M6 | 命令面板 ⌘P（会话 + 机器 + 命令，kero 的模糊打分）、空态、悬停控件 | `palette_panel.go`、`controls.go`、`elevation.go` |
| M4 | 设置窗口：左侧页列表 + 卡片上的分组面板 + 外观页（主题、五种 wash 缩略图）+ 分屏/折叠/丝滑配色 | `settings.go` |
| M3 | 浏览器端：删除标签栏、侧栏即会话、卡片、五态图标、消息 chip、计数 chips、手机抽屉，全部走生成的 token | `web/src/style.css`、`app.ts`、`fleet.css`、`tokens.css` |

### 验证

- `go vet ./...`、`go test ./...`：全绿（含新增 `internal/status` 与 `internal/uitokens` 测试）。
- `pnpm run check`（go vet + tsc）：通过。
- `pnpm run acceptance`：**30/30**（浏览器客户端：新建、关闭、断线、重连续接、只读、接管）。
- `pnpm run acceptance:relay-browser`：**24/24**（中继链路、手机触屏、只读拦截、接管、桌面被降级、会话在客户端退出后存活）。
- `pnpm run acceptance:relay`：**19 passed, 0 failed**（机器间中继链路；脚本的内层握手版本随协议升到 2）。
- 协议 `Version = 2`：`internal/protocol` 的 `Summary` 新增 `status / unread / message / kind / progress / since`，hub 在状态变化时广播。
- 预览图：`TF_RENDER=<dir> go test ./cmd/tfdesktop -run TestRender*`，覆盖浅/深 × 空态/单卡/分屏/舰队/只读/已退出/设置各页。

### 未做（有意留给下一版）

1. 原生通知与 Dock 徽标：mygo 0.5.5 没有通知 / 徽标 API（kero 用的 0.5.6 才有）。目前用窗口标题计数与侧栏脚替代。
2. 屏幕抓取式 agent 识别（herdr manifests）：daemon 里没有 VT，`mygo` 的 `vt` 是 internal 包；本轮只用不需要 VT 的信号。
3. `tf` CLI、agent skill/hook、`tf report`、会话在窗口退出后继续（常驻 daemon + 历史持久化）。
4. 快捷键重绑（actions 注册表已按 kero 的形态落了一半，尚未接到设置页）。
5. 指针处 tooltip、拖拽 pane 换位、Ghostty 主题库、配置文件（Ghostty 语法）。

### 0.5.1 · 分屏与快捷键对齐 kero

- **分屏是窗格，不是标签页**：`⌘D` / `⌘⇧D` 之后侧栏只保留一行（`workspaceLeaves`），行上聚合最紧急的窗格状态，提示里写窗格数；关掉一个窗格只把它移出分屏，shell 继续运行并重新获得一行。桌面端本地与远端会话两条列表都做了分组（`nav.go`）。
- **快捷键**：`⌘]` / `⌘[` 切换窗格、`⌘⌥←→↑↓` 跨分隔线、`⌘⇧↩` zoom、`⌘R` 重命名（刷新挪到 `F5`）、`⌘=` / `⌘-` / `⌘0` 字号。尚未接：`⌘⇧W`（结束会话，需要确认框）、`⌘⇧R`（原地重启 pane，需要重启语义）、`⌘⇧T`（在 home 新开）。
- 新增测试：`workspace_test.go`（一行、关窗格回一行、`⌘]`/`⌘[`、方向键、zoom、字号）。

### 0.5.2 · 通知、前台进程、快捷键重绑

- **通知**：mygo 升到 `v0.5.6-0.20261020063447-d5bcfdb3d75b`（带 `NewNotification` / `NotificationsSupported` / `SetBadgeCount`）。`reportStatuses` 在 waiting / error / done 三种跃迁时发通知（窗口前台且正是当前会话时跳过），点击切回会话；Dock 徽标显示 waiting 数；「设置 → 通知」四个开关，`恢复默认` 全开。决策逻辑抽成 `noticeFor`，`onNotify` 是测试缝。
- **前台进程**：`ptyProcess.Foreground()`（unix: `tcgetpgrp` + `/proc/<pgrp>/comm` / `kern.proc.pid`；Windows 返回空），session 每秒轮询并推给 tracker；协议 `Summary.Process`；桌面与浏览器在卡片头上显示非 shell 的进程 chip。
- **快捷键重绑**：`actions.go` 注册表（20 个动作）+ 解析/格式化（`cmd+shift+d` 存储形式，`⇧⌘D` 显示，`CmdOrCtrl+Shift+D` 菜单形式）；`runShortcuts` 统一分发；「设置 → 键盘」录制、冲突提示、⌫ 清除、恢复默认；`settings.Keybinds` 持久化；命令面板、空态按钮、菜单加速键同步。
- 新增测试：`notify_test.go`（5）、`actions_test.go`（7）、`session.TestForegroundNames`、`status.TestTheForegroundProgram…`。
