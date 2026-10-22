<p align="center">
  <img alt="Beacon" src=".github/assets/logo.png" width="140" height="140">
</p>

<h1 align="center">Beacon</h1>

<p align="center">
  <strong>一个终端中枢，随处掌控。静候呼唤。</strong>
</p>

<p align="center">
  <a href="https://github.com/CarlOpenLab/term-fleet/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/CarlOpenLab/term-fleet/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/CarlOpenLab/term-fleet/releases/latest"><img alt="Latest Release" src="https://img.shields.io/github/v/release/CarlOpenLab/term-fleet?color=6366f1"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue">
  <img alt="Go Version" src="https://img.shields.io/badge/go-%3E%3D1.24-00ADD8">
</p>

<p align="center">
  <a href="README.md">English</a> • <a href="README.zh_CN.md">简体中文</a>
</p>

---

**Beacon** 将分布在各台物理机、虚拟机与云服务器上的真实 PTY 会话，编织为一个随处可达、具备状态感知的终端中枢。

宿主机守护进程（`tfd`）直接托管真实 PTY 并通过单条多路复用 WebSocket 对外服务。基于 Mygo 构建的精致原生桌面客户端（浮动卡片式布局、弥散渐变水洗背景，纯原生渲染无需 WebView）、浏览器端以及手机端，均可同时附着到同一个会话中。手机上的终端 Tab，就是桌面上正在运行的同一个 Shell。

各机器通过公网中继（`tfrelay`）向外主动建连（Dial-Out），无需公网 IP、端口映射或自建 VPN，即可在外网随时随地掌控所有设备。

```
       桌面客户端                       手机 / 网页浏览器
  (原生 Mygo 桌面窗口)                 (自适应 Web 客户端)
           │                                   │
           │  wss://                           │  wss:// (首帧 Token 鉴权)
           ▼                                   ▼
   ┌────────────────────────────────────────────────────────┐
   │                  公网中继 (tfrelay)                     │
   │            仅路由数据流 · NAT / CGNAT 穿透              │
   └────────────────────────────────────────────────────────┘
                               ▲
                               │  wss:// (各节点主动向外建立连接)
            ┌──────────────────┴──────────────────┐
            │                                     │
     本地工作站 1                           远端服务器 2
┌───────────────────────┐             ┌───────────────────────┐
│       Agent (tfd)     │             │       Agent (tfd)     │
│   PTY / ConPTY 中枢   │             │   PTY / ConPTY 中枢   │
│   环形输出缓冲与重放   │             │   环形输出缓冲与重放   │
└───────────────────────┘             └───────────────────────┘
```

---

## 核心特性

- **多端同源附着（真正的一份 Shell）**  
  不是冷冰冰的远程桌面画质推流，而是实时的 PTY 字符流同步。基于「单写入者租约 + 多端观察」机制：手机在口袋里可以静默同步查看进度，不会因误触打乱桌面输入；想要敲命令时，一键即可接管输入权。
- **状态感知与注意力呼唤（"It tells you when one needs you"）**  
  Beacon 能自动识别终端标题、响铃（Bell）、命令 Spinner 以及程序主动报告的结构化状态（`OSC 7501`），提炼出 `working`（运行中）、`waiting`（等待输入）、`idle`（闲置）、`error`（报错）及 `exited`（已退出）。当长任务跑完、命令报错或后台 AI Coding Agent 等待你的授权时，Dock 角标与系统通知会第一时间呼唤你。
- **断开只是离开，绝非结束**  
  合上笔记本屏幕、离开 Wi-Fi 导致连接断开，只是客户端脱离附着，主机进程依然在环形缓冲区保护下持续运行。重新连上时，按字节偏移量精准增量续接，不丢失日志，也不产生刷屏风暴。
- **免公网端口，NAT / 穿透漫游**  
  所有受控节点均采用主动向外拨号（Dial-Out）设计。无论是办公室局域网、家里的 NAS 还是处于大内网（CGNAT）后的设备，都无需配置路由端口转发，安全省心。
- **高审美的原生桌面体验**  
  桌面端采用纯原生 Go 绘制，具备精致的浮动卡片视觉体系、弥散水洗流光配色、分屏智能继承当前工作目录（自动通过 `/proc` 与 `lsof` 嗅探）、`⌘P` 模糊命令面板与 `⌘J` 快速直达待办任务。

---

## 对比

| 能力项 | **Beacon** | SSH | tmux | tmate |
| :--- | :---: | :---: | :---: | :---: |
| **免入站端口 (NAT 穿透)** | **原生支持** (中继向外拨号) | 否 (需公网 IP / 端口映射) | 不适用 (本地工具) | 支持 (依赖第三方服务) |
| **电脑与手机同时在线** | **原生支持** (桌面 + 移动端) | 需第三方复杂配置 | 仅限终端内 | 仅提供简易 Web 查看 |
| **网络闪断增量续接** | **原生支持** (字节偏移重放) | 闪断即掉线 | 留在会话中 | 留在会话中 |
| **程序状态感知与呼唤** | **原生支持** (OSC 7501 / Bell) | 无 | 无 | 无 |
| **智能分屏继承路径** | **原生支持** (嗅探当前 cwd) | 手动处理 | 需写额外脚本 | 手动处理 |
| **原生现代化 GUI 界面** | **原生支持** (浮动卡片 / 调色) | 仅纯文本黑框 | 仅纯文本黑框 | 仅纯文本黑框 |

---

## 下载与安装

每个正式版本的全平台安装包均可在 **[Releases 页面](https://github.com/CarlOpenLab/term-fleet/releases)** 下载：

| 平台 | 安装包格式 | 包含内容 |
| :--- | :--- | :--- |
| **macOS** | `.dmg` | 通用二进制（原生支持 Apple Silicon M 系列与 Intel 芯片） |
| **Windows** | `Setup.exe` / `.zip` | x64 安装程序与免安装绿色版（底层由 ConPTY 驱动） |
| **Linux** | `.deb` / `.tar.gz` | Debian、Ubuntu 格式与便携 tar 归档（amd64 / arm64） |
| **CLI 工具** | `.tar.gz` / `.zip` | 独立守护程序 `tfd` (Agent) 与路由服务 `tfrelay` (Relay) |

> [!NOTE]
> 社区构建未购买商业证书签名：在 macOS 上首次打开如果提示受阻，右键点击 App 选择「打开」即可；在 Windows 上若触发 SmartScreen，点击「更多信息 → 仍要运行」。

---

## 快速上手

### 1. 本地 / 局域网模式

仅需在一台机器上启动，即可供本机桌面客户端或局域网内的手机浏览器访问：

```sh
# 源码构建
git clone https://github.com/CarlOpenLab/term-fleet.git beacon
cd beacon
pnpm install
pnpm run build

# 启动守护进程（在 127.0.0.1:7678 提供 Web 客户端）
./bin/tfd

# 启动原生桌面窗口（直接连接同一会话）
./bin/tfdesktop
```

在电脑浏览器打开 `http://localhost:7678` 即可使用；如需在局域网内用手机访问，添加参数运行 `./bin/tfd -addr 0.0.0.0:7678 -token <访问密钥>` 即可。

---

### 2. 通过中继随处漫游（多机跨公网）

#### 步骤 A：部署公网中继（只需一台带公网 IP 的轻量服务器）
```sh
./bin/tfrelay -addr 0.0.0.0:7698 \
              -client-token "YOUR_CLIENT_TOKEN" \
              -public-url "wss://relay.example.com/relay/agent" \
              -devices devices.json
```
*(中继内置了 Web 管理后台 `https://relay.example.com/admin`，支持在线注册机器、重置密钥、实时查看节点诊断日志)*。

#### 步骤 B：机器接入中继
```sh
./bin/tfd -relay "wss://relay.example.com/relay/agent" \
          -device-id "my-macbook" \
          -device-secret "从管理后台生成的设备密钥"
```

接入后，无论是用桌面客户端，还是手机直接打开 `https://relay.example.com/`，所有受控机器上的终端随时随地随你调用。

---

## 桌面端快捷键速查

| 快捷键 | 动作 |
| :--- | :--- |
| `⌘P` / `Ctrl+P` | 打开命令面板（模糊搜索会话、机器与操作） |
| `⌘J` / `Ctrl+J` | 快速跳转至下一个**需要你注意/授权**的会话 |
| `⌘T` / `Ctrl+T` | 新建终端（自动继承当前分屏所在的工作目录） |
| `⌘D` / `Ctrl+D` | 垂直左右分屏 |
| `⌘⇧D` / `Ctrl+Shift+D`| 水平上下分屏 |
| `⌘[` / `⌘]` | 在当前平铺的分屏之间移动焦点 |
| `⌘⇧[` / `⌘⇧]` | 在不同会话组之间切换 |
| `⌘1` … `⌘9` | 按序号快速跳转会话 |
| `⌘⇧↩` / `Ctrl+Shift+Enter` | 聚焦放大当前分屏 / 恢复平铺 |
| `⌘R` / `Ctrl+R` | 重命名当前终端标题 |
| `⌘W` / `Ctrl+W` | 关闭当前分屏（若为最后视图则结束其后台进程） |

*所有快捷键均可在「设置 → 键盘」中自定义或录制冲突检测。*

---

## 程序主动状态通知 (`OSC 7501`)

除了自动嗅探响铃与标题变化，CLI 脚本或 AI Agent 还可以直接输出 `OSC 7501` 转义序列，将进度或等待状态精准呈现在边栏：

```sh
# 上报等待用户授权确认：
printf '\033]7501;state=waiting:kind=permission:msg=%s\033\\' "$(printf '是否同意执行此操作？' | base64)"

# 上报执行进度：
printf '\033]7501;state=working:progress=45:msg=%s\033\\' "$(printf '正在构建镜像...' | base64)"

# 恢复闲置就绪状态：
printf '\033]7501;state=idle\033\\'
```

---

## 三条核心设计原则

1. **附着（Attach）不是创建（Create）**  
   客户端断开（锁屏、合盖、掉线）只是单方面的离开。终端进程与其输出历史完好无损地驻留在宿主机上，重连时仅拉取差量数据。
2. **单写入者租约机制**  
   物理终端的 PTY 只有一个尺寸与标准输入。如果桌面与手机同时抢占输入或改变行高列宽，画面必然乱颤。因此同一会话同时只允许一个控制器写入，其余客户端以只读模式实时同步镜像，轻触即可平滑接管。
3. **关闭标签页即销毁进程**  
   桌面端点击标签页的 `×` 或按 `⌘W` 会向机器发送终止信号并清理该会话。网页端则细分为了「关闭查看视图（不杀进程）」与「结束进程」两个独立按钮。

---

## 配置与日志路径

桌面客户端的配置文件与诊断日志持久化在各操作系统的标准目录下：

- **macOS**: `~/Library/Application Support/beacon/`（向下兼容 `term-fleet`）
- **Windows**: `%AppData%\beacon\`
- **Linux**: `~/.config/beacon/`

### 环境变量
- `BEACON_CONFIG_HOME`: 自定义配置与日志保存目录（最高优先级）。
- `TERM_FLEET_CONFIG_HOME`: 历史环境变量，向下兼容。

---

## 本地开发与贡献

```sh
# 1. 安装前端依赖并打包资源
pnpm --dir web install
pnpm run build:web

# 2. 运行 Go 单元测试套件
pnpm run test        # 或直接执行 go test ./...

# 3. 静态检查与 TypeScript 类型校验
pnpm run check       # 内部执行 go vet ./... && tsc --noEmit

# 4. 端到端端浏览器与中继连通性测试
pnpm run acceptance:all
```

### 仓库代码结构
```
cmd/tfd/               无头宿主机守护进程（管理 PTY、提供 WebSocket）
cmd/tfdesktop/         原生桌面客户端（基于 Mygo GUI）
cmd/tfrelay/           公网中继路由服务（连接配对、设备管理、鉴权）
internal/session/      PTY 抽象层（Unix PTY + Windows ConPTY）与输出环形缓冲
internal/hub/          WebSocket 会话扇出与租约仲裁逻辑
internal/agent/        向中继反向拨号的 Agent 客户端模块
internal/relay/        中继核心路由表与机器注册管理
internal/uitokens/     桌面 UI 视觉 Token（弥散渐变、卡片色值）
web/                   网页端与移动端前端（TypeScript + xterm.js）
deploy/                Systemd 守护进程单元、Caddy 代理配置与部署运维手册
```

---

## 许可证

本项目遵循 [MIT License](LICENSE) 开源协议。
