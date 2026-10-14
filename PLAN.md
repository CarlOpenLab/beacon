# term-fleet · 计划

> 一句话：**把终端从「某台机器的某个窗口」里解放出来。**
> 终端进程跑在机器上，画面按需分发给局域网里的 PC 和手机；多端各自 attach，
> 进程不随视图消失，谁控制谁干活可以随时交接。

## 1. 三件事

| # | 需求 | 落地形态 |
|---|---|---|
| 1 | **区域网分发终端** | 每台机器跑一个守护进程 `tfd`，持有 PTY 会话，监听局域网端口 |
| 2 | **PC 端终端应用** | Mygo（Go + 系统 WebView）打包的桌面应用；同一进程既是 UI 也是区域网的服务端 |
| 3 | **移动端控制** | 同一个 web 应用在手机浏览器打开（PWA 加到桌面）；tab 列表与主机**一比一** |

「一比一」的定义（可验收）：手机列出的每一行 = 主机上一个存活会话；
**每个存活会话恰好一行，顺序与 PC 一致**，刷新不漂移。

## 2. 先例与取舍

读过两个同生态的先例，**只取经验，不抄代码**：

- **tela**（Carl 自己的，`carl-github/tela`）——四条硬经验直接继承：
  1. 单一 WebSocket 多路复用，消息带 `sessionId`，不为新功能另开连接
  2. `attach/detach`（视图在不在看）与 `create/close`（进程在不在）**必须分离**
  3. 输出带游标，重连只补缺口，不重放全量
  4. **任何路径都不给 PTY 发 0 尺寸**（`cols=0` 会让 vim/htop 认为窗口消失，重挂时花屏）
- **endlessblink/termfleet** —— TermControl 手机 companion 的做法值得抄：
  bridge 默认只绑 localhost，`TC_HOST=0.0.0.0` 才让同网段手机连上；
  并把「phone 与 PC 的 pane 清单一比一」写成验收门槛。
- **IKEASven69/dsh-termfleet** —— 只取一条：**只读 / 可操作两档**的会话级权限，
  外加限时与留痕。它的「同意总线」复杂度我们不要。

## 3. 架构

```
        PC 应用（Mygo: Go 二进制 + WebView）       手机浏览器（PWA）
        ┌───────────────────────────┐            ┌──────────────┐
        │  tfd 内核（同一进程内）      │            │  web 前端     │
        │  session / pty / hub      │            └──────┬───────┘
        └─────────────┬─────────────┘                   │
                      │  WebSocket（唯一一条）           │
                      └───────────────┬─────────────────┘
                                      │  局域网
                            ┌─────────┴──────────┐
                            │  主机 A (macOS)     │
                            │  tfd  ── PTY × N    │
                            └────────────────────┘
                            ┌────────────────────┐
                            │  主机 B (Windows)   │  ← 第二个守护进程，
                            │  tfd  ── ConPTY × N │     协议相同
                            └────────────────────┘
```

**一台机器 = 一个 `tfd` + N 个 PTY 会话。**
客户端可以直连某台机器（区域网）；将来要在多台机器之间聚合时，
再加一个 hub 角色复用同一份协议——现在不做，但协议按「以后能聚合」来设计。

## 4. 技术选型

| 层 | 选型 | 理由 |
|---|---|---|
| 服务端 | **Go**（单二进制） | PC 端既然用 Mygo，后端同语言就没有 Node sidecar |
| PTY | `creack/pty`（unix）/ ConPTY（Windows） | 接口抽象出来，Windows 后补 |
| WebSocket | `github.com/coder/websocket` | 现代、context 驱动、无回调地狱 |
| 前端 | Vite + TypeScript + `@xterm/xterm` | 与 tela 同栈，复用踩坑结论 |
| 前端产物 | `go:embed` 进二进制 | 单文件分发，支持完全离线 |
| PC 外壳 | `github.com/egoist/mygo` | 需求指定 |

**为什么不是 Node/Tauri**：需求指名 Mygo；且 Go 单二进制同时解决
「桌面应用 + 区域网服务端 + 手机页面的静态服务」，不需要三套运行时。

## 5. 协议（v1 草案）

单一 WebSocket，文本帧，每条 = 一个 JSON。`t` 是判别字段。

客户端 → 服务端

| `t` | 载荷 | 语义 |
|---|---|---|
| `hello` | `v` | 协议版本握手 |
| `list` | `reqId` | 要一份会话清单 |
| `create` | `sessionId, cols, rows, cwd?, shell?, title?` | 新建（同 id 存活则复用） |
| `attach` | `sessionId, cursor` | 开始看；补发 `cursor` 之后的输出 |
| `detach` | `sessionId` | 不看了，**会话保留** |
| `input` | `sessionId, data` | 写入 stdin（需控制权） |
| `resize` | `sessionId, cols, rows` | 改尺寸（需控制权） |
| `claim` | `sessionId` | 申请控制权 |
| `release` | `sessionId` | 交还控制权 |
| `close` | `sessionId` | **真正结束进程** |

服务端 → 客户端

| `t` | 载荷 |
|---|---|
| `hello` | `v, sessions[], host{hostname,platform,arch}` |
| `sessions` | `sessions[]`（清单变化时主动推） |
| `created` / `attached` / `closed` / `exit` | 生命周期 |
| `output` | `sessionId, cursor, data` |
| `role` | `sessionId, role: controller\|viewer, controller` |
| `error` | `code, message, sessionId?, reqId?` |

`SessionSummary = { sessionId, pid, cwd, cols, rows, alive, cursor, title, controller, viewers }`

游标用**字节偏移**（不是消息序号）：`PtySession` 维护一条线性缓冲和单调递增的
写入偏移，`attach` 时从客户端报的 `cursor` 开始补发。比逐消息 seq 更好推理，
断线重连天然只补缺口。

## 6. 多端策略（tela 没解决的那部分，必须一开始就定）

一个 PTY 只有一组 `cols/rows`，而 PC 和手机屏幕差一个数量级。三种做法：

| 方案 | 行为 | 判断 |
|---|---|---|
| 尺寸取所有端最小 | 各端都不截断 | ❌ PC 被手机拖成窄屏，而且两端 resize 会互相打架 |
| 尺寸跟最后操作者 | 谁打字听谁的 | ❌ 切来切去，每次都要重排，观感最差 |
| **控制端制**（选这个） | 任一时刻只有一个 `controller`：只有它能 `input`/`resize`；其余是 `viewer`，只读镜像 | ✅ 语义干净，且「交接控制权」是显式动作 |

**尺寸归控制端。** viewer 按 PTY 的真实 `cols/rows` 渲染，用缩放去适配自己的屏幕，
**不改变内容尺寸**——这样手机看到的是和 PC 完全一样的画面（只是变小），
而不是被重排过的另一种画面。手机上可以捏合放大，需要打字时先 `claim`。

控制权规则：
- 新 attach 默认 `viewer`；第一个 attach 者若无人持有则自动成为 `controller`
- `claim` 抢占：旧 controller 降级为 viewer 并收到 `role` 通知
- controller 断开 / 空闲超时（默认 30 min 无输入）→ 自动释放
- `input` / `resize` 来自非 controller → 回 `error{code:"not_controller"}`，**不静默丢弃**

### M0 落地时定的两个细节

- **`close` 要控制权**。`detach` 才是「我不看了」，任何人随时可用；`close` 是
  「把这个进程杀了」，对所有端都不可逆，所以归控制端。非控制端会收到
  `not_controller`，客户端据此提示「先接管再关闭」。
- **`data` 走 base64**（Go 侧即 `[]byte`）。PTY 输出不保证是合法 UTF-8，用字符串
  字段会让 `encoding/json` 把非法字节静默改写成 U+FFFD，`cat` 一个二进制文件就露馅。

## 7. 里程碑

- **M0 走路骨架** ✅
  Go 守护进程：PTY + 会话（环形缓冲 + 游标）+ 单 WS 多路复用 + 控制端制；
  一个 Go 写的测试客户端跑通「create → attach → input → output → 断线重连补发」。
- **M1 PC 端可用** ✅
  Vite + xterm 前端 → `go:embed` → Mygo 外壳：多 tab、新建/关闭、快捷键、复制粘贴。
- **M2 移动端**
  同一前端的响应式版：清单页（一比一）+ 会话页 + 控制权交接 + 软键盘辅助键条。
- **M3 加机器**
  Windows 主机（ConPTY）、多主机清单、按机器分组的 tab。
- **M4 加固**
  鉴权（token）、WSS、审计留痕。

### M1 落地时定的三件事

- **前端由 daemon 自己托管**，Mygo 外壳只是「把窗口指向本机的 loopback」。
  于是 PC 壳、浏览器标签页、M2 的手机加载的是同一份产物、同一个源，
  不需要 CORS、不需要另一套打包。代价：外壳进程必须活着，
  PTY 是它的子进程，关窗口即结束会话（与关掉一个终端模拟器一致）。
- **前端不用框架**（Vite + TS + xterm，无 Vue）。这份 bundle 也是手机在局域网
  里要下载的东西，体积直接决定首屏；xterm 之外的部分不到 10 KB。
- **`go:embed` 的产物目录保留 `.gitkeep`**：干净检出下 `go build` 必须能过，
  否则 `go test ./...` 在 CI 里会因为没有 `dist/` 而失败。构建脚本负责补回标记。

## 8. 还没定

- ~~项目名最终是 `term-fleet` 还是 `termfleet`~~ → 定为 `term-fleet`（远端
  `github.com/CarlOpenLab/term-fleet`，Go module 同名）
- ~~PC 端是否要保留「一个进程既是服务端又是 UI」~~ → M1 定为「是」，
  见上面 M1 的三件事第一条
- 手机端要不要包成原生壳（uni-app / RN），还是 PWA 就够
- 是否需要多主机 hub（M3 之后再定）
- 标签栏的辨识度：同一目录下的多个 shell 标题相同。现在靠 tooltip 带
  `sessionId` / `cwd` 区分，是否要给 shell 注入 OSC 标题（或加序号）待定

## 9. 安全底线（现在就写下来）

- `tfd` 默认**只绑 localhost**；要局域网可见必须显式打开（对齐 TermFleet 的做法）
- （M0 已落地：`validateBinding` 在非环回地址且没给 `-token` 时**直接拒绝启动**，
  不做「警告后继续」。）
- 一等公民是「执行任意命令」的能力：开放局域网前必须有 token；公网一律 WSS
- 文件相关能力如果以后加，路径必须过白名单（借 tela 的 `FsService` 教训）
