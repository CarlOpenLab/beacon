# term-fleet UI 重设计 · 共享规格（SPEC）

> 这份文件是三个设计方向（A/B/C）共同的唯一事实来源。三份 HTML 必须遵守同一份产品事实、同一份 mock 内容、同一份交付契约，只在**视觉语言与布局策略**上分化，这样用户才能公平对比。

---

## 0. 任务与交付

- 产出：三个可点击的高保真 HTML 原型，文件放在本目录下
  - `direction-a-quiet-precision.html`
  - `direction-b-command-deck.html`
  - `direction-c-editor-native.html`
- 每个文件**完全自包含**：不加载 CDN、不使用外部字体/图片/构建步骤，纯 HTML + CSS + 原生 JS + 内联 SVG 图标。
- 用户已确认：**深色 + 浅色两套主题**、**可点击交互原型**（不是静态稿）、名称仍是 `term-fleet`、文案可以改写。
- 目标是"确认方向后直接落地到 `web/src`"，因此**所有布局都必须能用 CSS/DOM 实现**（不用 canvas 画 UI、不依赖 JS 布局计算），并标注清楚每个交互对应真实 app 的哪个状态。

---

## 1. 产品事实（不许编造的功能）

term-fleet 把一个机器的 PTY 会话通过一条 WebSocket 分享给桌面端、浏览器和手机端。同一份网页客户端由 **daemon（直连，局域网）** 或 **relay（中继，跨网）** 提供，两种模式下界面的差别只有：中继模式多一个"机器列表"层级、一个连接/令牌界面，状态栏多一个中继状态。

### 1.1 界面分区（现有结构，重设计可改布局但不能丢功能）

```
┌──────────────────────────────────────────────────────────┐
│ header  42px: brand(● term-fleet) · banner · 新建终端 ⌘T  │
├──────────┬───────────────────────────────────────────────┤
│ sidebar  │ tabs: [● carl ×][● term-fleet-dev ×] +        │
│ 236px    ├───────────────────────────────────────────────┤
│ 机器分组  │ terminal pane (xterm, 只读显示, 无内边距)      │
│ + 会话行  │                                               │
│          ├───────────────────────────────────────────────┤
│          │ composer: [textarea] [发送]                    │
│          ├───────────────────────────────────────────────┤
│          │ keys: Esc Tab Ctrl Alt ↑ ↓ → ← C-c C-d | - / ~ │
├──────────┴───────────────────────────────────────────────┤
│ status 26px mono: [已连接] 主机信息 · 会话信息              │
└──────────────────────────────────────────────────────────┘
```

### 1.2 功能清单（重设计必须全部保留，可换表达方式）

**header**：品牌点 + 名称；中间是 banner（同一时刻最多一条，带一个动作按钮）；右侧"新建终端 ⌘T"。
桌面壳里 header 是窗口拖拽区（`-webkit-app-region: drag`），所以**按钮必须留出 no-drag 区域**——设计时把可点区域集中成右侧一组即可。

**banner 的四种状态**（原文案，可改写）：
| 状态 | 文案 | 动作按钮 |
|---|---|---|
| 机器离线 | `carls-Mac-mini 离线，重连后将自动恢复` | 无 |
| 会话已退出 | `会话已退出，可读取输出后关闭标签` | `关闭标签` |
| 只读（被占用） | `只读：该会话正被 c4 控制` | `接管`（点击前有确认对话框） |
| 只读（本机不允许被控制） | `只读：carls-Mac-mini 不允许其他客户端控制它的终端` | `接管` |

**sidebar**：标题"主机会话" + 总数徽标；直连模式只有一台机器（不显示机器分组），中继模式按机器分组（机器名 + 在线状态 + 折叠；离线机器置灰、有"离线"标记；会话行悬停出现"结束"按钮，结束是不可逆操作）。会话行两行信息：
- 第一行：状态点 + 标题（标题 = 会话名 / cwd 末段 / sessionId）
- 第二行（等宽小字）：`s2 · 70×30 · pid 25745 · 控制中 c1 · 已退出` 之类拼装字段
空状态：`主机上还没有会话` / `没有会话` / `中继上还没有登记任何机器`。

**tabs**：每个标签 = 状态点（绿=自己是控制端 / 灰=旁观 / 红=已退出）+ 标签名；中继模式下标签前有一个机器名 pill；右侧 `×` 关闭（**关闭 = 停止观看，不杀进程**，与"结束"语义不同）；末尾 `+` 新建。
快捷键：`⌘T` 新建、`⌘W` 关闭当前标签、`⌘Tab`/`⌘⇧Tab` 前后切换、`⌘1..9` 跳第 N 个标签。

**terminal pane**：xterm 渲染，只读显示面（`disableStdin`），选中文本时浮动出现"复制"chip；只读标签在右上角有"只读"角标。

**composer**：唯一的键盘输入通道（输入在本地组好，Enter 整条发出）。Enter 发送、Shift+Enter 换行、↑/↓ 单行时翻本标签的发送历史。占位文案随状态变化：
`输入命令，回车发送（Shift+回车换行）` / `尚未创建会话` / `会话已退出` / `只读：等待接管` / `连接已断开，重连后即可发送`。
禁用态与键条一起被禁用（键条半透明）。

**keys 键条**：`Esc Tab Ctrl Alt ↑ ↓ → ← C-c C-d | - / ~`；`Ctrl`/`Alt` 是**粘滞键**（点亮，下一次按键与之组合，`C-c` 就是 Ctrl+C）；指针为精确设备（鼠标）时键条缩小约一半高度。

**status bar**：等宽小字三段：连接状态 chip（`已连接` / `连接中…` / `已断开` / `机器离线` / `中继离线` / `鉴权中…`）、主机信息（`carls-Mac-mini.local · darwin/arm64 · 4 个会话`）、当前会话（`s2 · 控制中 · 70×31`，角色取值 `控制中` / `只读` / `—`，无会话时 `未选中会话`）。

**toast**：右下角浮出，3.6s 后淡出。示例：`已被接管（c4），你现在是只读` / `该会话由其他端控制，输入已被拒绝` / `已复制` / `中继已断开` / `服务端协议版本 2，客户端期望 1`。

**连接中继页**（中继模式下需要令牌时全屏覆盖）：标题"连接中继" + 原因说明（如 `需要访问令牌才能连接中继` / `访问令牌无效，请重新输入`）+ 两个字段（`中继地址` 默认 `wss://relay.example.com/relay/client`；`访问令牌` password，空则不提交，Enter 在地址框里跳到令牌框）+ `连接` 按钮 + 说明文字"令牌只保存在这台设备的浏览器里，通过首帧发送，不会出现在 URL 或访问日志中。"。连接过程有"鉴权中…"态。

**手机端**（≤720px）：侧栏变抽屉（左上汉堡按钮打开，点遮罩关闭）、键条为触控放大版并避开 home indicator 安全区、点击区域更大、状态栏可折行。`@media (pointer: coarse)` 时会话行/标签的行高更大，"结束"按钮常驻可见。

### 1.3 设计上已知的"丑"（用户的原话：偏丑）

当前实现的问题，重设计应正面解决，但不要靠堆装饰解决：
1. 全部是 1px 平涂边框，没有任何层次与深度；hover 只是换背景色。
2. 圆角不统一（4/5/6/7/9/12px 混用），间距无网格（7px/9px/11px 随手写）。
3. 主按钮颜色浑浊（`#2b3b63` 底 + `#3a4d7d` 边），既不像强调色也不像中性色。
4. 侧栏第二行等宽小字 10.5px 灰字，信息塞在 `·` 里，难扫读。
5. tab 栏几乎没有存在感（透明底 + 2px 下划线），标签与终端区没有边界。
6. 空状态是一句居中灰字浮在巨大黑区里，没有任何引导。
7. composer 看起来像网页表单（圆角输入框 + 灰色按钮），不像终端 app。
8. 键条 14 个等权重的灰色小方块，噪音大、没有分组、没有图标。
9. 状态栏 26px 内塞三段等宽小字，对比度低。
10. 全站零图标，全部靠文字和 `·`；中文用 13px + 默认字距，可读性一般。

---

## 2. 共享 mock 内容（三份必须一字不差地用同一份数据）

为了让三套方向可以公平对比，请使用**完全相同**的假数据、文案与终端内容，只在呈现方式上分化。

### 2.1 主机与会话

```json
[
  {
    "id": "local", "label": "carls-Mac-mini.local", "online": true,
    "host": { "platform": "darwin", "arch": "arm64" },
    "sessions": [
      { "id": "s2", "title": "carl", "cwd": "~", "cols": 70, "rows": 31, "pid": 25745, "controller": "c1", "alive": true,  "role": "controller" },
      { "id": "s5", "title": "term-fleet-dev", "cwd": "~/Desktop/carl-github/term-fleet", "cols": 132, "rows": 38, "pid": 31299, "controller": "c4", "alive": true, "role": "viewer" },
      { "id": "s9", "title": "deploy", "cwd": "~/srv/api", "cols": 120, "rows": 30, "pid": 28810, "controller": null, "alive": false, "role": null }
    ]
  },
  {
    "id": "build-box", "label": "build-box.lan", "online": true,
    "host": { "platform": "linux", "arch": "amd64" },
    "sessions": [
      { "id": "b1", "title": "go test ./...", "cwd": "/srv/term-fleet", "cols": 200, "rows": 50, "pid": 8812, "controller": "c1", "alive": true, "role": "controller" }
    ]
  },
  {
    "id": "macbook-air", "label": "macbook-air", "online": false,
    "host": { "platform": "darwin", "arch": "arm64" },
    "sessions": []
  }
]
```

侧栏计数徽标 = `4`。中继模式默认连的是 `relay.example.com`；直连模式（本机）场景下不显示机器分组，只列 `local` 的 3 个会话。

### 2.2 打开的标签（3 个）

1. `carl` — local/s2 — **控制中**（绿点）— 当前激活
2. `term-fleet-dev` — local/s5 — **只读**（灰点，banner 提示被 c4 控制）
3. `go test ./...` — build-box/b1 — 控制中（中继模式下标签前显示机器 pill `build-box`）

### 2.3 终端内容（逐字使用；`{}` 内是语义标注，用于着色，不要打印出来）

**标签 1（carl，控制中，活跃）**

```
{cwd}~/Desktop/carl-github/term-fleet{/cwd} {dim}via{/dim} {accent}v24.20.0{/accent}
{prompt}❯{/prompt} pnpm run check
{cmdhead}> term-fleet@0.4.8 check{/cmdhead}
{cmdhead}> go vet ./... && pnpm --dir web run typecheck{/cmdhead}

{cmdhead}> term-fleet-web@0.4.8 typecheck{/cmdhead}
{cmdhead}> tsc --noEmit{/cmdhead}
{dim}· 1.24s{/dim}
{prompt}❯{/prompt} git status -sb
{branch}## main...origin/main{/branch}
{modified} M{/modified} cmd/tfdesktop/reach_test.go
{modified} M{/modified} internal/hub/hub_test.go
{modified} M{/modified} internal/relay/relay_test.go
{prompt}❯{/prompt} go test ./internal/hub/ -run TestAttach
{ok}ok{/ok}  {path}github.com/carl/term-fleet/internal/hub{/path}  {dim}0.42s{/dim}
{prompt}❯{/prompt} {cursor}
```

**标签 2（term-fleet-dev，只读 — 顺便展示失败输出）**

```
{prompt}❯{/prompt} go test ./...
{err}--- FAIL: TestRosterCallbackGuard (0.01s){/err}
{err}    hub_test.go:214: read loop closed before the callback ran{/err}
{err}FAIL{/err}  {path}github.com/carl/term-fleet/internal/hub{/path}  {dim}0.38s{/dim}
{err}FAIL{/err}
{prompt}❯{/prompt} {cursor}
```

**标签 3（build-box，go test）

```
{prompt}❯{/prompt} docker compose -f deploy/compose.yml ps
{colhead}NAME      IMAGE            STATUS       PORTS{/colhead}
{ok}tfrelay{/ok}   term-fleet:dev   Up 3 hours   0.0.0.0:8443->8443/tcp
{ok}tfd{/ok}      term-fleet:dev   Up 3 hours   127.0.0.1:7678->7678/tcp
{prompt}❯{/prompt} {cursor}
```

### 2.4 状态栏内容

`已连接` / `carls-Mac-mini.local · darwin/arm64 · 4 个会话` / `s2 · 控制中 · 70×31`（切到标签 2 时 `s5 · 只读 · 132×38`，切到标签 3 时 `b1 · 控制中 · 200×50`）。中继模式下还要能表达 `中继` 与鉴权中的状态。

### 2.5 空状态 / 首次进入

`主机上还没有会话`（直连）或 `中继上还没有登记任何机器`（中继）。空状态不是一个死胡同：**重设计要在这里给出下一步动作**（新建终端、连接中继、看看快捷键），这是用户明确要求重做的部分。

---

## 3. 交付契约（每个 HTML 都必须满足）

### 3.1 尺寸与视图

- 桌面视图基准 **1440×900**，必须在这个尺寸下**无滚动条**地完整呈现（`html,body{height:100%;overflow:hidden}`）。
- 手机视图基准 **390×844**。
- URL 参数（画布通过 iframe 传入，必须支持）：
  - `?theme=dark|light`（默认 dark）
  - `?screen=main|empty|connect`（默认 main）
  - `?frame=phone`（渲染 390×844 手机外壳，居中放在中性背景上）
- 页内 Tweaks 面板也要能切换这三项。

### 3.2 主题

- `<html data-theme="dark">`，浅色为 `data-theme="light"`，所有颜色走 CSS 变量，**只切换属性**。
- 浅色不是把深色反相：请为浅色单独定 ttokens（背景近白、层次靠极淡灰和 1px 线、终端底色可以仍是深色——像终端 app 那样保留"深色纸面"是允许且通常更好的选择，但要在文档里说明）。
- 文字对比度：正文 ≥ 4.5:1；终端内容 ≥ 4.5:1（终端是主体，别为了氛围压暗）。

### 3.3 必须实现的交互（可点、有状态变化）

1. **标签切换**：点标签；激活标签的视觉必须明显；终端内容、状态栏第三段、banner 跟着变。
2. **新建终端**：header 按钮 / tab 栏 `+` / 空状态主按钮 → 新增一个标签并激活（模拟：出现新标签 `新终端` + 终端里出现新会话起始行）。空状态点它之后必须离开空状态。
3. **关闭标签**：`×` → 标签消失并激活相邻标签；全部关闭后回到空状态。
4. **侧栏**：机器分组折叠/展开；点会话行切换/打开标签；会话行 hover 出现"结束"（弹出确认式气泡即可，不必真删）；离线机器置灰 + 离线标记；只读会话有明确标记。
5. **只读 → 接管**：切到 `term-fleet-dev` 时出现 banner `只读：该会话正被 c4 控制` + `接管` 按钮；点按钮 → 状态变为控制中（banner 消失或变成成功提示，标签点变绿，composer 解禁，toast `已接管`）。
6. **composer**：输入文字 Enter 提交 → 终端里追加一条"命令 + 假输出"的命令组（体现它会把整条命令发出去）；`↑`/`↓` 翻历史；禁用态的占位文案按 §1.2 切换。
7. **键条**：Ctrl / Alt 粘滞点亮，再点一次取消；点击键有按下反馈。
8. **复制 chip**：在终端区选中文本（或提供演示按钮）→ 浮出 `复制` chip，点击后 toast `已复制`。
9. **toast**：至少 3 种触发方式（接管 / 复制 / 只读被拒），4s 自动消失，可叠放。
10. **连接页**：从 Tweaks 或（中继场景）侧栏入口进入；令牌为空点"连接"→ 行内错误 `访问令牌无效，请重新输入` + 聚焦令牌框；填了 → `鉴权中…` 约 1.2s → 进入主界面。
11. **主题切换**：深/浅即时切换，不能出现对比度崩坏。
12. **手机视图**：汉堡按钮开合抽屉侧栏 + 遮罩；键条放大且避开安全区；点击区域 ≥ 44px。
13. 所有可点元素都要有 **hover / active / focus-visible** 三态。

### 3.4 Tweaks 面板（页内控件）

- 默认**收起**为一个小的胶囊按钮（放在视口左下角，如 `Tweaks`），点开是紧凑面板；因为画布上三个方向会同屏，常驻面板会互相干扰。
- 控件：主题（深/浅）、屏幕（主界面/空状态/连接页）、视图（桌面/手机）、侧栏（显示/隐藏）、只读演示（开/关）、终端密度（紧凑/宽松，可选）。
- 面板本身用该方向的视觉语言做，不要用浏览器默认控件样式。

### 3.5 与画布通信（postMessage）

```js
// 收到画布广播：
window.addEventListener('message', (e) => {
  const m = e.data
  if (!m || m.type !== 'tf-set') return
  if (m.theme)  setTheme(m.theme)     // 'dark' | 'light'
  if (m.screen) setScreen(m.screen)   // 'main' | 'empty' | 'connect'
})
// 加载完成后通知父窗口（可选，便于画布显示就绪状态）：
parent.postMessage({ type: 'tf-ready', direction: 'A' }, '*')
```

### 3.6 代码与结构要求

- `<style>` 与 `<script>` 内联在同一个文件；不要模块化 import；不要 `type="module"`。
- 结构：`<html data-theme="dark">` → `<div class="app">` → 各分区。屏幕级容器加 `data-screen-label`（如 `data-screen-label="主界面"` / `"空状态"` / `"连接中继"`）。
- CSS 变量集中在 `:root`，按 `--bg / --bg-raised / --bg-sunken / --line / --fg / --fg-dim / --accent / --ok / --warn / --err` + 方向自己扩展的 token 命名，并给出 `--space-*`、`--radius-*`、字号 token。
- 间距用 flex/grid + `gap`；不要靠空白文本节点或逐元素 margin 排版。
- 内联 SVG 图标（1.5px stroke、currentColor）自己画简单几何形状即可，不要用 emoji。
- 终端内容用 HTML 行渲染（**不要**真的去加载 xterm）。
- 文件顶部写一段注释：方向名、一句话主张、关键 token 表、与真实 app 的落地映射要点。
- 文件较大，建议分 2–4 次写入（先 head + token + 主界面，再追加其他屏幕与脚本），保证每次写入都是完整可解析的结构。

### 3.7 视觉质量红线

- 不要 AI 味：大面积渐变、发光泛紫、emoji、"圆角卡片 + 左侧彩色描边"烂俗套、Inter/Roboto 字体。
- 字距不要滥用；中文不要 `letter-spacing` 拉到 0.7px 那种（小字可以 0.02em 以内）。
- 圆角、间距、颜色都要成体系，不随手写数值。
- 中文排版：正文行高 1.6–1.75，字号 ≥12px（终端等宽 12.5–13.5px 是例外，允许 1.35–1.5 行高）。
- 不要用假数据把界面塞满充数；每一条信息都要有存在的理由（对应 §1.2 的真实字段）。

### 3.8 字体栈

```css
--font-ui: -apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", "Noto Sans SC", system-ui, sans-serif;
--font-mono: "SF Mono", "JetBrains Mono", ui-monospace, Menlo, Consolas, "Liberation Mono", monospace;
```

---

## 4. 三个方向的分工（必须明显不同，且各自成立）

三个方向共享产品事实、mock 内容、契约；**分化点必须落在布局策略、信息层级、控件形态、排版与色彩上**，不能只是换个强调色。

### 方向 A · Quiet Precision（克制精准）
血统：Linear / Raycast 的安静与精确。
- 近中性深灰底（`#0b0c0e` 级），层次靠**极淡的层级差 + 发丝线**（`rgba(255,255,255,.06~.1)`），阴影极轻。
- 唯一强调色（低饱和靛蓝），其余全靠文字透明度分级（100/70/50%）。
- 布局主张：**键盘优先**。header 压到 40px，只留品牌 + 一个"新建终端"主按钮；标题栏不再承担信息职责。
- tab 栏改成 Linear 式紧凑填充标签（圆角、无下划线、激活态浮起一级），与终端区之间有清晰边界。
- 侧栏 260px 扁平单列：机器是分组小标题，会话是行；等宽 meta 只留 `70×31` 与角色两个字段，其余进 hover tooltip。
- 终端区做"纸张"感：外层留 12–16px 内边距，终端内容有明确可读宽度与行高。
- 输入区做成 Raycast 式**单行命令条**：左侧提示符图标 + 输入 + 右侧 `⌘↵ 运行` 提示，整体一个圆角条，不像表单。
- 键条默认折叠（`⌨ 按键` 胶囊，点开浮出面板）。
- 状态栏缩成右下角一条极细信息带（三段用分隔点而不是三个盒子）。
- 性格关键词：安静、精确、一眼看懂、克制到近乎冷淡。

### 方向 B · Command Deck（指令甲板）
血统：Warp 的"命令块" + Termius 的连接管理。
- 更冷更深的底（`#07090c`），强调色用青绿，警告用琥珀；颜色承担状态语义（连接/只读/退出/失败各有色）。
- 布局主张：**命令即对象**。终端里的每条命令 + 输出是一个"命令块"：左侧 3px 状态色条、悬停浮出操作（复制命令 / 复制输出 / 折叠）、块间有间距；成功/失败/进行中在块头有状态标记与耗时。
- 右侧 280px **会话检视栏**：sessionId、cwd、尺寸、pid、控制端、所属机器、连接时延（RTT ms 是示意值，标注为示意），可折叠——这是 Termius 血统的落地。
- tab 栏带机器 pill，标签用"下缘连接线 + 状态色"表达归属与角色。
- 侧栏保留机器分组与状态点，会话行给"控制中 / 只读"徽标而不是小字。
- 输入区是**多行 composer**：左侧 `❯`、上方可展开的历史下拉、右下主按钮 `运行 ⌘↵`；键条按 [修饰键] [方向键] [符号] 三组分区，组间有分隔与不同底色。
- 状态栏分段为 chip 组（连接 / 机器 / 会话 / 中继），等宽字体，右侧带连接质量指示。
- 性格关键词：专业、信息密度高、像仪表台的驾驶舱、一屏掌握全局。

### 方向 C · Editor Native（编辑器原生）
血统：Zed / VS Code 的编辑器骨骼 + 一点大胆的现代感。
- 高对比双主题：深色近黑、浅色近白，强调色鲜明（紫罗兰或薄荷，二选一，别两个都用）。
- 布局主张：**把会话当文件、把机器当工作区**。顶部 44px 是应用自己的工具条（客户端跑在浏览器标签页里；桌面的 tfdesktop 是 Mygo 原生窗口、不是 WebView，所以**不要**画 macOS 流量灯之类的系统窗口 chrome，这一条优先于任何"桌面 app 感"的想象），标签做成 pill，激活态有轻微浮起与阴影。
- 侧栏 288px，顶部有一个**会话过滤框**（演示用，输入即过滤；对应真实 app 未来的 ⌘K 能力，标注为"提议"）。
- 浮层是本方向的招牌：`⌘K` 命令面板（列出会话/机器/操作，可键盘上下选择）、toast、复制 chip 都用半透明 + backdrop-blur 的薄玻璃，层次干净不过度发光。
- 手机视图做完整：抽屉 + 底部大输入区 + 放大键条。桌面版提供 `⌘K` 面板入口按钮。
- 性格关键词：有性格、精致、现代、编辑器原生感、允许一点惊艳。

---

## 5. 交付前自检清单（每个方向逐条过）

- [ ] 1440×900 无滚动条；390×844 手机视图不溢出。
- [ ] `?theme=` `?screen=` `?frame=` 三个参数都生效；Tweaks 面板能切同样三项。
- [ ] 深色与浅色都逐屏检查过，没有对比度崩坏或看不见的边界。
- [ ] 10 项交互（§3.3）全部可点且状态真的会变，不是装饰。
- [ ] 终端内容是 §2.3 的逐字内容，行/列对齐良好（不是等宽字体错位、不是断行成两行）。
- [ ] 侧栏/标签/状态栏/终端四处的状态互相一致（切换标签时同时变化）。
- [ ] 没有 emoji、没有外部资源、没有 console 报错。
- [ ] 中文文案读起来自然，没有 `·` 堆砌。
- [ ] 文件顶部的设计说明注释写清楚了方向主张与 token 表。
