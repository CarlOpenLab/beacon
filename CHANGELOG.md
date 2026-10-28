# Changelog

Notable changes per release. The release workflow lifts the section matching the
pushed tag straight into the GitHub release notes, so keep the headings exactly
`## <version>`.

## 0.7.11

**Beacon 0.7.11: 实时路径跟踪与分屏包裹树 — Live CWD tracking and split tab brackets**

- **Live CWD tracking follows the shell through every directory change.** macOS now polls the kernel directly via `__proc_info(PROC_PIDVNODEPATHINFO)` in ~2µs with zero subprocess overhead, updating `cd` movements in real time in the card header and sidebar instead of freezing on the initial launch path.
  — 实时路径跟踪跟随 Shell 目录变化：macOS 下采用内核直接系统调用 `__proc_info` 毫秒级读取前台进程当前目录，零子进程开销。在终端中执行 `cd` 切换到任何路径，顶部卡片栏与侧边栏目录标签立即实时同步，不再停留在首次打开路径。
- **Split tabs are unified with main tabs, framed by symmetric dashed brackets.** Split child panes now share the exact same 34px row height, 13.5px font size, rename interactions, and hover close buttons as main session rows, with the hollow idle ring removed. Split groups are wrapped in a clean, symmetric bracket (`┌┄┄┄` on the first pane, `├┄┄┄` on middle panes, `└┄┄┄` on the last pane) using delicate 1.0px hairline dashed lines, and all pane labels align strictly in a single column.
  — 分屏 Tab 全面对齐主 Tab 并以对称虚线树包裹：分屏子项统一 34px 行高、13.5px 字号、双击重命名与悬停关闭按钮，移除了空闲状态下的空心圈圈。分屏组以对称的精致微点虚线括号完整包裹（首项 `┌┄┄┄`、中间项 `├┄┄┄`、尾项 `└┄┄┄`），文字标签纵向严格对齐。

## 0.7.10

**Beacon 0.7.10: 图标不再比别人大一号 — The app icon fits its slot**

- **The machines of the rail are pixel pets now.** Every machine wears one of thirty-two hand-drawn 16×16 pets — four families of eight, from companion animals to the brand's own lamp — so the shape says which machine is which while the colour only follows the tile: at rest, hovered, chosen. A machine's name picks its pet by a stable hash, and right-clicking the rail tile is the picker itself — a panel of the drawings, no menu in between; the machine card offers the same among its actions. A 史莱姆 is chosen by being seen, not by its name, and a chosen machine wears the active blue whatever its state, offline included. A machine that needs you or has failed is told by its tile's wash alone — one fact, in one place, instead of a second copy in the glyph's colour.
  — 底栏的机器现在是像素宠物：三十二只手绘 16×16 小家伙，四个家族各八只，从陪伴动物到品牌自己的那盏灯。形状认机器，颜色只跟随交互状态——静置、悬停、选中。机器名经稳定哈希认领宠物（本地网络追加的后缀不改变它）；右键底栏圆片就是选择面板本身，中间不再隔一层菜单，机器卡片则在菜单里提供同样的面板——史莱姆是被看见的，不是被念出名字的；选中就是选中，离线也一样是那身 active 蓝。需要处理或失败的机器只靠底色表达，状态这件事始终只有一个出处。
- **The app icon keeps the transparent margin its neighbours have.** The dark tile was drawn edge to edge, and an OS draws an app icon at the size of its slot — so the plate filled the slot completely and Beacon read about a tenth larger than the apps beside it on the Windows desktop and in the macOS Dock. The tile now covers 80% of the canvas, the proportion of Apple's grid and of the icons next to it, and the mark inside is drawn at that scale rather than scaled after the fact. Tests hold the margin, its proportion and its centring, so the icon cannot go back to full-bleed.
  — 暗色底板此前满画布绘制，而系统按图标槽位尺寸绘制应用图标，底板便占满整个槽位：在 Windows 桌面和 macOS Dock 里，Beacon 都比旁边的应用大出约一成。现在底板只占画布 80%，与 Apple 图标网格及邻居的比例一致，内部图形按该比例直接绘制而非事后缩放。测试锁住留白、占比与居中，图标不会退回满版。

## 0.7.9

**Beacon 0.7.9: Windows 图标小尺寸更清晰 — Sharper Windows icon at small sizes**

- **Give the app icon stronger small-size optics.** Windows derives its 16–256px icon variants from one master image; the lamp and beams now carry more visual weight so they remain clear in Explorer and desktop shortcuts. macOS keeps using the same shared app icon.
  — 加强应用图标在小尺寸下的视觉重量：Windows 从同一主图生成 16–256px 图标，放大的灯头和光束在资源管理器与桌面快捷方式中更清晰；macOS 继续使用共用应用图标。

## 0.7.8

**Beacon 0.7.8: 文字粘贴不再被吞 — Text paste reaches the program**

- **A text paste into a program that asks for paste events now arrives.** Windows cannot carry a terminal's OSC 5522 paste event through ConPTY — the console host drops it instead of handing it to the program — so omp and other agents that enable paste events for images never received pasted text, while images still worked. The desktop writes a text paste itself, with the terminal's paste events off, so the text goes as an ordinary paste: bracketed when the program asked for that, and read by every program. The paths an image upload pastes go the same way.
  — Windows 的 ConPTY 不会把终端的 OSC 5522 粘贴事件交给程序（控制台宿主直接丢弃），所以为图片开启粘贴事件的 omp 等 agent 收不到文字粘贴，而图片照常可用。桌面端现在自己把文字按粘贴写入（终端侧关闭粘贴事件，程序要求时带括号），任何程序都能读到；图片上传后粘贴的路径也走同一条路。

## 0.7.7

**Beacon 0.7.7: 修复 Windows 文字粘贴 — Restore Windows text paste**

- **Keep plain-text paste working when another clipboard format is malformed.** Windows now falls back to the native Unicode text clipboard when reading all advertised formats fails, while image and file pastes keep their existing handling.
  — Windows 剪贴板同时提供异常富文本格式时，桌面端改用原生 Unicode 文本兜底，恢复本机与远程终端的文字粘贴；图片和文件粘贴流程保持不变。
- Verified the desktop paste path, Windows cross-compilation, and the Go test suite.
  — 已验证桌面粘贴路径、Windows 交叉编译和 Go 测试套件。

## 0.7.6

**Beacon 0.7.6: 底部机器栏上下留白一致 — Balanced machine rail spacing**

- **Remove the separator above the machine rail.** Circular machine controls retain their borderless state fills, and now sit with equal 8-point space above and below instead of extra padding at the bottom. The resize handle respects the same rail bounds. Layout tests verify the spacing at narrow, default and wide sidebar widths.
  — 移除底部机器栏上方的分隔线，保留圆形图标的无边框状态背景。图标上下各留 8 点距离，不再额外偏向底部；拖动句柄同步避开机器栏，布局测试覆盖窄、默认和宽侧栏。
- Includes the local CLI image-paste fix from v0.7.5.
  — 包含 v0.7.5 的本机 CLI 图片粘贴修复。

**Full Changelog**: https://github.com/CarlOpenLab/beacon/compare/v0.7.5...v0.7.6

## 0.7.5

**Beacon 0.7.5: 本机图片粘贴交给 CLI 处理 — Native image paste for local CLI agents**

- **Local image-only pastes now use the same gesture as Gloss.** Instead of uploading a screenshot and inserting its quoted path as text, the desktop forwards raw Ctrl+V to the local foreground program. Image-aware CLI agents can then read the original system clipboard and attach the image themselves. Text pastes, explicit file lists and read-only restrictions retain their existing behavior.
  — 本机剪贴板只有图片时，桌面端按 Gloss 的方式向前台程序发送原始 Ctrl+V，让支持图片的 CLI agent 自己读取系统剪贴板并创建附件，不再把截图路径当普通文字插入。文字粘贴、显式文件列表及只读限制保持原有处理。
- **Remote images still upload to the shell's machine.** Remote programs cannot read the local clipboard, so they continue receiving an uploaded file path. Regression tests cover the local image gesture, mixed text/image clipboard content, file uploads, remote images and read-only panes.
  — 远程程序无法读取本机剪贴板，图片仍上传到 shell 所在机器后传入文件路径。新增回归覆盖本机图片按键、图文混合剪贴板、文件上传、远程图片与只读窗格。
- **Snapshot upload metadata before starting background work.** The upload worker no longer reads machine roster and session metadata from UI-owned state, fixing a data race while session lists update during a paste.
  — 上传开始前在界面线程获取机器平台、连接和会话信息，避免上传协程与会话列表更新并发读取界面状态，修复相关数据竞争。

**Full Changelog**: https://github.com/CarlOpenLab/beacon/compare/v0.7.4...v0.7.5

## 0.7.4

**Beacon 0.7.4: 底部机器切换栏与图片粘贴修复 — Machine rail and image paste fixes**

- **Switch machines from a centered, scrollable rail.** The desktop sidebar now shows the selected machine's sessions, with circular machine controls along the bottom. Right-click a machine to choose a persistent icon. Machine names omit local network suffixes and distinguish duplicate names. Command+1 through Command+9 switch machines in rail order.
  — 桌面侧栏按所选机器展示会话，底部圆形机器栏居中排列，机器较多时支持横向滚动。右键可更换图标并保存；机器名去除本地域名后缀，同名机器补充标识。Command+1 至 Command+9 按栏内顺序切换机器。
- **Separate selection, hover and machine states without borders.** Active controls use stronger fills; needs-you, pending, running, completed, failed and offline states have distinct treatments. Session status marks lead their rows, and choosing a machine can take you to its waiting session.
  — 选中与悬停采用不同强度的无边框填充，需要你、创建中、运行中、已完成、失败和离线状态使用不同配色。会话状态标记前置，切换机器时可定位到等待回答的会话。
- **Keep search, settings and sidebar controls together.** The top controls appear in search/settings/fold order, align to the sidebar's right edge while expanded, and remain available when folded. Fixed button sizes and additional title space prevent compression and title overlap during resizing.
  — 顶部按搜索、设置、折叠排列，展开时靠侧栏右侧对齐，折叠后仍可操作。固定按钮尺寸并预留完整标题空间，改善拖动时的图标压缩和标题重叠。
- **Negotiate image and file paste support on both desktop transports.** The local connection now sends a hello handshake, and remote connections announce file upload and compression capabilities. This fixes screenshots being recognized but rejected with an empty capability list. Pasted files are uploaded to the shell's machine and their quoted paths are inserted into the terminal. Regression coverage exercises the local clipboard-to-upload-to-input path and remote handshake.
  — 本地连接补齐 hello 握手，远程连接声明文件上传及压缩能力，修复截图已被读取却因能力列表为空而遭拒绝的问题。图片或文件上传至 shell 所在机器后，以加引号的路径粘入终端；新增本地粘贴完整链路及远程握手回归测试。

**Full Changelog**: https://github.com/CarlOpenLab/beacon/compare/v0.7.3...v0.7.4

## 0.7.3

**Beacon 0.7.3: 侧栏只留正在用的机器 — the column keeps the machines in use**

- **A machine with nothing under it folds into one row.** The column used to give every machine a header, so a fleet of ten was a page of names with nothing under them. A machine now keeps its header only when it has sessions, when one of its sessions is waiting on an answer, or when it is the machine the panes are on; the rest are counted in 「其他 N 台」, which opens into ordinary headers and takes them away again. Hiding a machine outright still works, and hiding a machine the person is in never happens on its own: an empty group under the panes is the loudest answer to 「我点到哪了」.
  — 侧栏原来给每台机器一个标题，十台机器就是一页只有名字的空行。现在只有有会话的、有会话在等你回答的、以及窗格正踩着的那台保留标题，其余收进「其他 N 台」，展开就变回普通标题。手动隐藏照旧。
- **The current machine is the one on screen, not the last click.** Picking a machine this window has no shell on leaves the terminal where it was, so that click never became the truth about where a person is — but the column took it as the current machine and kept a quiet one out of the other row for the rest of the session. It asks the active tab now, and falls back to the pick only when there is nothing on screen.
  — 点一台这窗口没有 shell 的机器时，窗格不会跟着换；可侧栏把这次点击当成了「你正在用的那台」，于是那台空机器一直留在外面。现在按当前标签页所属的机器算，屏幕上什么都没有时才用点选的那台。
- **Folding a group is movement, and by hand it is the movement you asked for.** A group's rows fold with 180 ms and the rows below ride up over them. The flag that says a person folded it was being spent on the very frame that drew the click, so the animated curve never ran: it was always the instant one. Clicked folds animate again, window-driven ones still land at once, and 减弱动态效果 takes every one of them to its place in the first frame.
  — 分组收起时那 180 毫秒原来从没走过：记着「这是手点的」的那个标记在画这一帧时就被用掉了，于是永远走的是瞬间那条。现在手点的带动画，窗口自己触发的仍然立刻到位，系统开了减弱动态效果的一律瞬间到位。
- **Open, the other machines are a group with a line above them.** Folded, 「其他 N 台」 is a summary and a line under it would claim a second group; open, its rows are machines and without the line they run into the sessions above. The line lives inside the fold, so it leaves with the content instead of flashing.
  — 收起时它是一句摘要，画线等于宣称这是第二个分组；展开时里面是真机器行，没有线就和上面的会话行连成一片。线画在折叠里面，收起时跟着内容一起走，不会先闪一下。

## 0.7.2

**Beacon 0.7.2: Windows 的卡片保留圆角 — Windows keeps its card corners**

- **A card never goes square.** 0.7.1 made the card `window − 8`, which is exactly parallel; on Windows the window rounds at 8, so that is 0 — a square card inside a rounded window, which reads as no card at all and pinches the wash at the corner instead of widening it. The card now keeps a floor of 8: as round as the window itself, with the frame widening by a little over a pixel at the corner rather than the card losing its shape. Linux (12 − 8 = 4) comes up to the same floor.
  — 0.7.1 让卡片取 `窗口 − 8`，严格平行；Windows 的窗口圆角是 8，于是算出 0 —— 圆角窗口里一张方角卡片，看着像没有卡片，而且转角处的留白被挤窄而不是变宽。现在卡片有 8 的下限：和窗口本身一样圆，角上的留白只宽一像素多一点，而不是把卡片的形状去掉。Linux（12 − 8 = 4）同样抬到这条下限。
- **macOS is asked which version it is.** The window corner moved from 10 (macOS 15 and earlier) to 28 (26, Tahoe); 0.7.1 hard-coded the modern one, which would over-round the card on an older Mac. The version is read from the kernel at startup, so Tahoe still gets 20 and older releases get the floor of 8 instead of a card rounder than its window.
  — macOS 的窗口圆角从 10（15 及更早）变成了 28（26，Tahoe）；0.7.1 写死的是新值，在旧系统上会把卡片做得比窗口还圆。现在启动时向内核读系统版本：Tahoe 仍是 20，旧版本落到 8 的下限，而不是一张比窗口更圆的卡片。

## 0.7.1

**Beacon 0.7.1: 卡片重新圆回来，并按窗口的圆角对齐 — Cards keep their rounding, sized to the window's corner**

- **The workspace card's corner is the window's minus the frame, at the window's real radius.** 0.7.0 fixed the alignment by assuming the window corner was 10 and shrinking the card to 4 — which is what a 10px window needs, and macOS 26 is not that window: it rounds at about 28. The card is 20 now (28 − 8), so it is rounder than the 14 it started with *and* the 8px wash keeps one width all the way round. Windows 11 (8) puts the card at 0 and Linux (12) at 4; the arithmetic is the same rule in all three. The card radius is no longer a standalone token, it is derived — so a future system change moves it instead of silently pinching the corner.
  — 0.7.0 把窗口圆角当成 10、把卡片缩到 4，那是 10px 窗口该有的答案；macOS 26 不是那个窗口，它是约 28。现在卡片是 20（28 − 8），比最初的 14 还圆，同时 8px 留白在角上和边上一样宽。Windows 11（8）算出 0，Linux（12）算出 4，三处是同一条规则。卡片圆角不再是一个独立 token，而是推导出来的——系统以后变圆，它跟着走，而不是悄悄把角挤薄。
- **A light card gets a 1px hard edge.** The window's own edge is a hard system line; a card lifted only by shadows reads as a soft grey smudge at the corner — right curve, wrong material. It is drawn as a zero-blur shadow rather than a border so it cannot take a pixel out of the card's inside.
  — 浅色卡片补了 1px 硬边。窗口那圈是系统画的硬线，只靠阴影浮起来的卡片在角上是一团软灰——弧度对了、质感不对。用零模糊阴影而不是 Border，避免它像内边距一样把卡片内容挤进去 1px。

## 0.7.0

**Beacon 0.7.0: 远程更接近本地 — paste images across machines, compress the wire, name every build — Beacon：图片能粘到对面，链路会压缩，每台机器的版本都看得见**

- **An image or a file on the clipboard becomes a file on the machine the shell runs on.** The terminal used to hand a paste to the program as text, which is nothing at all when the clipboard holds a screenshot: a shell does not read the clipboard protocol, so the paste silently died. Both clients now take an image or a file, upload it over `file.put`/`file.chunk`/`file.done` into a private temp directory on that machine (32 MB ceiling, name sanitized, one upload in flight), and paste the returned path quoted for that machine's shell — POSIX or PowerShell. The desktop hooks the terminal's `OnPaste`; the browser reads clipboard files. A read-only pane says why instead of swallowing the paste.
  — 剪贴板里是截图时，终端原来把粘贴当文本交给程序，而普通 shell 不读剪贴板协议，于是这次粘贴静悄悄地消失了。现在两端都会取出图片或文件，用 `file.put`/`file.chunk`/`file.done` 传到那台机器的私有临时目录（上限 32 MB、文件名清洗、同一时刻一个上传），再把返回的路径按那台机器的 shell 加引号粘进去（POSIX 或 PowerShell）；桌面端挂在终端 `OnPaste` 上，浏览器读取剪贴板文件；只读窗格会说明原因，而不是吞掉。
- **The wire is compressed when both ends know how.** Every Data field worth more than a few hundred bytes is zlib-compressed, negotiated in the hello and only ever sent to a peer that named `deflate`. Terminal output is the bulk of a fleet's traffic and it repeats itself; a megabyte replay that used to cost about 1.8× its bytes after two layers of base64 now costs a fraction of that. An old hub and an old client keep the exact wire they had.
  — 超过几百字节的 Data 现在都做 zlib 压缩，在 hello 里协商，且只发给声明了 `deflate` 的对端。终端输出是流量的大头且高度重复，一次原本因两层 base64 膨胀到约 1.8 倍的重放，现在只占其中一小部分；旧 hub 与旧客户端仍然走完全相同的帧。
- **Every machine says which build it runs, and the same daemon enrolled twice is refused.** The handshake, the relay roster and the machine tooltip now carry the version and an instance id (hostname, pid, start). A machine that is behind is visible where you act on it; a second enrollment of the same running daemon is rejected at the relay with the id it duplicates, instead of showing one machine's sessions twice.
  — 握手、中继名册和机器提示现在都带版本与实例 id（hostname、pid、启动时间）。落后的机器在你要动手的地方就能看见；同一个运行中的 daemon 被登记两次时，中继会用已在线的那台机器的 id 拒绝第二条链路，而不是把一台机器的会话列两遍。
- **`tfd service install` registers the daemon with the OS, and one line installs it.** systemd user unit on Linux (with linger), a launchd agent on macOS, a logon task on Windows; `uninstall`, `restart` and `status` alongside. `scripts/install-member.sh` / `.ps1` download the latest `beacon-cli` release and run it, so a machine joins in one command and updates by running the same one again. Released binaries now carry their version (`tfd version`).
  — Linux 用 systemd 用户单元（并开启 linger），macOS 用 launchd agent，Windows 用登录任务；另有 `uninstall`、`restart`、`status`。`scripts/install-member.sh` / `.ps1` 下载最新 `beacon-cli` 后调用它，一台机器一条命令加入，重跑同一条命令即更新；发行二进制现在带有版本号（`tfd version`）。
- **The scrollback a reconnecting client replays is four times larger, and truncation is said out loud.** The per-session ring is 1 MiB; when a client was away longer than it holds, the desktop now says the screen may be incomplete instead of leaving a full-screen program looking merely broken (the browser already said so). The relay client also has one bounded writer queue: the window's thread never waits on a relay that stopped reading, and a queue that fills ends the link so the next one resumes from the cursor rather than losing bytes silently.
  — 每个会话的环形缓冲提升到 1 MiB；离开太久时桌面端现在会提示画面可能不完整，而不是让全屏程序看起来只是坏了（浏览器端原本就会提示）。中继客户端也只有一个有界发送队列：窗口线程不再等待一个停止读取的中继；队列满时结束链路，让下一条从游标续上，而不是静默丢字节。
- **Tokens for people, scoped to machines and expiring on their own.** The relay can mint a named credential limited to a few machine ids with a lifetime, list it in the admin panel, and revoke just that one — so a phone or a colleague no longer needs the fleet-wide token, and taking access back does not sign everyone else out.
  — 中继可以签发限定到几台机器、带有效期的具名凭据，在管理页列出、单独撤销——手机或同事不必再共用一个全舰队令牌，收回权限也不会把其他人一起踢下线。
- **Corner radii line up at the window edge, and a remote notification names its machine.** Workspace cards inset 8 px from a window whose corner is the system's now use a radius concentric with it, so the frame between the two curves keeps the same width at the corners as along the edges. A notification about a shell on another machine says which one, so a remote program cannot read as a local one.
  — 距离窗口边缘 8 px、而窗口圆角由系统绘制的工作区卡片，现在改用与窗口同心的半径，两条弧之间的留白在转角处和边上一样宽。来自另一台机器的通知会写明是哪台，远端程序无法冒充本机。
- **Not in this release.** A member is not updated automatically — run the install line again, or `tfd service install` with the new binary (its sessions end with it). The reattach fix for a truncated replay is a bigger ring and an honest sentence, not a server-side screen snapshot; remote paths still open on the machine you are sitting at, not on the far machine.
  — 本版不含：成员不会自动更新（重跑安装命令，或用新二进制再执行 `tfd service install`，会话会随之结束）；截断重放的修法是更大的环形缓冲加一句实话，而不是服务端画面快照；远端路径仍在本地机器打开，而不是在那台远端机器上。

## 0.6.8

**Beacon: a tab for a session that is gone is closed, not called exited — Beacon：会话已经不在了的标签，直接关掉，而不是说它「已退出」**

- **A tab opened on a session the hub no longer has is closed, with a sentence.** The panel draws the last session list its machine pushed, so a row can outlive its shell; clicking one opened a tab, the attach was answered `no_session`, and the window read the attach that never came as an exit — "会话已退出，最后几行仍可读取" over a blank pane, a sentence about a final line that does not exist. That tab is removed now, the stale row goes with it, the machine's list is asked for again, and the status bar says the session is gone. A tab that did watch a shell end keeps its banner and its output.
  — 「会话」面板画的是一台机器最后一次推来的列表，所以一行可能比它的 shell 活得久：点它会开出一个标签，attach 得到 `no_session`，而窗口把「没有 attach」读成了「已经退出」，于是在一个空面板上写「会话已退出，最后几行仍可读取」——一句最后一行为空的话。现在这种标签会被关掉，过期的行也一起消失，并重新向那台机器要一次会话列表，状态栏说明这个会话已经不在了；真正看着 shell 结束的标签仍然保留横幅和最后输出。
- **Session lists are counted in the diagnostic log.** Every list a machine pushes logs its machine and size, so a row that will not go can be told apart from a list that never arrived.
  — 机器每次推来的会话列表都会在诊断日志里记下机器与条数，于是「行不肯消失」和「列表根本没来」可以区分开。

## 0.6.7

**Beacon: a close closes the tab, even from an older client — Beacon：旧客户端发出的关闭，也不会再留下「已退出」**

- **A close dismisses the tab unless it says it is 结束.** 0.6.6 put "this is a tab close" on the close frame, and that only worked when the device that closed it was already on 0.6.6: a device still on 0.6.5 sent the same bare frame for 关闭标签 and 结束, so its closes reached the hub with nothing on them and every other client kept a 已退出 pane. The hub now takes a bare close as a tab close; 结束 sends `keep`, and that is the only close that leaves the tab showing the final output. One consequence, for one release: a 0.6.6 client's 结束 arrives bare, so its watchers drop the tab — the client that pressed it keeps its own.
  — 0.6.6 让 close 帧带上「这是关标签」，但只对已经在 0.6.6 上的设备生效：还停在 0.6.5 的设备，关闭标签与结束发出的是同一帧不带任何标记的 close，于是别的客户端照样留下「已退出」的面板。现在 hub 把不带标记的 close 当作关标签处理，只有明确带 `keep` 的结束才保留标签继续显示最后输出（0.6.7 的客户端会发）。过渡期有一个代价：0.6.6 客户端按「结束」时发的是不带标记的 close，观看端会关掉标签——按下的那一端自己的标签仍然保留。
- **Every close says what it meant, in the diagnostic log.** A close now logs its session, `keep` and `dismiss`, so a tab that outlives its shell can be traced to the side that sent the frame — a stale client, or a daemon on a machine that has not been updated.
  — 每次 close 现在都会在诊断日志里记下 session、`keep` 与 `dismiss`，于是「标签比 shell 活得久」可以直接追到发帧的那一侧：是没更新的客户端，还是某台机器上的旧 daemon。

## 0.6.6

**Beacon: a closed tab is closed everywhere — Beacon：关掉的标签，每台设备上一起关掉**

- **Closing a tab takes the tab with it on every client.** The × and ⌘W ended the shell and then left a 已退出 tab on the machine the shell runs on, and on every phone or browser watching it — a close and an exit were the same frame on the wire, and an exit is deliberately kept so its last lines can be read. A close now says it is a tab being closed, and the hub repeats that on the exit frame: each client drops its tab with the process, so the machine that hosted the shell does not keep a dead pane nobody there closed. 结束 sends the same close without the flag, and that is still the act that leaves the tab holding the final output.
  — 桌面端 × 与 ⌘W 结束进程后，会在运行这个 shell 的机器和其他正在观看的手机/浏览器上留下一枚「已退出」标签：close 与 exit 在协议上是同一帧，而 exit 是刻意保留的，好让最后几行还能读。现在关闭标签会带上「这是关标签」的标记，hub 把它带在 exit 帧里广播，各客户端随进程一起移除自己的标签，主机上不会再留一个没人关的死面板；「结束」仍发送不带该标记的 close，也就是保留标签读最后输出的那个动作。
- **The browser's × now ends the session too, after asking.** The mobile and web clients read a tab close as "stop watching", so closing there left the shell running and the desktop's tab alive. Closing a tab now means what it means on the desktop — the pane and the shell go together, everywhere — and it asks first, because the kill is irreversible for every other client and a thumb is easy to misplace. A tab whose shell has already exited is still only a view to dismiss.
  — 浏览器/手机端此前把关闭标签当作「停止观看」，关掉后 shell 继续跑、桌面端的标签也还在。现在点 × 与桌面端同义（窗格和 shell 一起走，且对所有客户端生效），并且会先确认——对其他人不可恢复，手指又容易点偏。已经退出的 shell 仍只是关掉视图。

## 0.6.5

**Beacon: a remote paste is this keyboard's, and a watched screen fits — Beacon：远程粘贴用本机剪贴板，观看屏幕适配窗格**

- **Ctrl+V at another machine's shell pastes this machine's clipboard.** The terminal used to send `^V` to the far end, where a Windows console or Claude Code read it as "paste the clipboard here" — the panel that opened belonged to the other machine, not the person typing. Remote tabs now bind `Ctrl+V` / `Ctrl+Shift+V` to the local clipboard (a performable binding: an empty clipboard still reaches the program as `^V`), and local shells keep their platform's keys untouched. The browser client also routes a paste made over the terminal into the composer instead of losing it in xterm's display-only textarea.
  — 控制其他机器时按 Ctrl+V 现在粘贴的是本机剪贴板。之前远端标签会把 `^V` 发给对端，Windows 控制台或 Claude Code 会把它当成“粘贴那边的剪贴板”，弹出的面板属于另一台机器。远程标签现在把 Ctrl+V / Ctrl+Shift+V 绑成本机粘贴（剪贴板为空时仍把 `^V` 交给程序），本机 shell 的按键不变；浏览器端在终端上粘贴也会进入 composer，不再被 xterm 的只读输入框吞掉。
- **A watched shell lays out at its own size, and can be scaled to fit the pane.** A viewer used to fit the replay to its own pane, so a full-screen program's lines wrapped here and not on the machine that wrote them, and every later cursor move landed a row off. Both clients now keep the shell's reported columns and rows (the replay is written for that screen) and, with the new 观看时缩放适配 / “Fit while watching” switch, shrink the text just enough to put the whole screen inside the pane; controlling is never scaled. The switch is on by default, skips screens that almost fit (under ~10% off), and stops at a readable 7.5 pt — a pane smaller than that still clips. Desktop: 设置 → 终端; browser: the gear's popover, remembered per device.
  — 观看端现在按远端 shell 上报的行列排版（重放就是按那块屏幕写的），并用新增的「观看时缩放适配」开关把字号缩到整屏刚好放进窗格；控制时永不缩放。开关默认开启，差距小于约 10% 时不缩，字号下限 7.5 pt，再小仍会裁切。桌面端在设置 → 终端，浏览器在齿轮菜单里，按设备记住选择。此前按本机窗格重排会导致全屏程序换行位置不同、之后每次光标移动都偏一行。
- **The read-only sentence lives in the card's header.** On the desktop, taking or releasing the lease used to add a 34 px strip under the header; the strip took its height out of the terminal, so the same shell lost its last rows just because control had moved. The sentence and its button now share the header row — replacing the descriptive chips while it shows — and the terminal keeps the same rectangle in both states.
  — 桌面端把只读横幅并进卡片头，替代原来的 34px 独立横条；控制/释放不再改变终端高度，同一块屏幕不会因为控制权变化而少掉最后几行。

## 0.6.4

**Beacon: the session list is dragged like a sortable list — Beacon：侧栏会话改成 sortable 式拖动**

- **A dragged row now previews the order it is making.** The row picked up leaves an empty slot where it would land, a copy follows the pointer, and the rows it passes slide out of the way; dropping writes the order, while Escape or letting go outside the rows puts it back. The browser client draws the same interaction with pointer events and a 160 ms FLIP instead of the browser's drag image, and decides the landing place from layout positions, so a list that is still sliding cannot change its mind under a pointer that has not moved.
  — 拖动的会话行现在就是新顺序的预览：原位置留出空槽，副本跟着指针，被经过的行滑开让位；松手才写入顺序，Esc 或丢在列表外则原样退回。浏览器端改用 Pointer Events 与 160ms FLIP 画同一套交互（浏览器自带的拖拽图像跟不上实时重排的列表），落点按布局坐标判定 —— 正在滑动的列表不会在指针不动时自己改主意。
- **The motion no longer bounces, and the list stays quiet under the pointer.** Rows move with a short spring that does not overshoot, a dragged row's copy is a lifted card rather than another row passing through, and hover backgrounds and the close × are suppressed while rows slide under the pointer. A drag that is cancelled or dropped outside leaves no trace in the saved order.
  — 动画不再弹：行用一段不回弹的短弹簧让位，被拖那枚的副本有卡片底和阴影，不再像另一行从列表里飘过；行从指针下经过时不闪 hover 与关闭 ×。取消或丢在列表外的拖动不会在保存的顺序里留下痕迹。

## 0.6.3

**Beacon: drag the fleet into your order, and let the terminal name itself — Beacon：把会话拖成自己的顺序，让标题自己说话**

- **The project is called Beacon.** The window, the browser page, the relay's admin panel and the docs now say Beacon throughout, the desktop bundle is `dev.carl.beacon`, and the service files follow the new name. A settings directory that already exists under `term-fleet` is still the one used — an update is not a reason to lose a font, a relay or a shortcut — and `TERM_FLEET_CONFIG_HOME` still works beside the new `BEACON_CONFIG_HOME`. README has a Simplified Chinese edition.
  — 项目正式改名为 Beacon：窗口、浏览器页面、中继管理页和文档统一换名，桌面 bundle 标识为 `dev.carl.beacon`，服务文件也跟着改。已有的 `term-fleet` 设置目录仍继续使用——升级不该弄丢字体、中继或快捷键；`TERM_FLEET_CONFIG_HOME` 与新的 `BEACON_CONFIG_HOME` 并存。README 补了简体中文版。
- **Drag a session row into the order you want.** The sidebar of both clients was fixed to the daemon's oldest-first list; now the desktop and the browser each remember their own arrangement, per machine. A row dragged over another lands above or below it by which half of the row the pointer is on, with an accent line marking where it will go; dragging never picks the row, and a shell the order has not seen yet joins at the end. The desktop keeps it in `settings.json`, the browser in `localStorage`, and a filtered list moves rows without disturbing the ones it is hiding.
  — 侧栏会话现在可以拖动排序：两个客户端原先都固定按 daemon 的最早优先顺序，现在桌面端和浏览器各自、按机器记住自己的排列。拖到另一行上半段就插在它前面、下半段插在它后面，强调色横线标出落点；拖动不会选中该行，顺序里没见过的会话排在末尾。桌面端写进 `settings.json`，浏览器写进 `localStorage`；筛选状态下拖动也不会打乱被隐藏的行。
- **An agent names the shell by naming its terminal.** The daemon now reads the title a program sets with `OSC 0` / `OSC 2` and uses it as the session's name when nobody has renamed the shell — which is how Claude Code and other AI CLIs say which task they are on. The animated spinner in front is taken off, so the name does not flicker with every frame; an explicit rename still wins, and a program that clears its title gives the name back to the program and directory. A set title is broadcast like any other state change, so the phone, the browser and the desktop change together.
  — AI 用「给终端起标题」来命名自己的任务：daemon 现在解析程序写入的 `OSC 0` / `OSC 2` 标题，在没人手动重命名时把它作为会话名——Claude Code 这类 AI CLI 正是这样说明当前任务的。标题前面的动画 spinner 会被去掉，名字不再逐帧闪动；显式重命名仍然优先，程序清空标题后名字回落到程序名与目录。标题变化会像状态一样广播，手机、浏览器和桌面一起更新。
- **The browser client speaks the protocol the daemon speaks again.** 0.6.2's browser bundle still announced version 1 to a daemon that requires version 2, so the page could load and then fail to show a single session. It now sends 2 and names both versions if they ever disagree. While fixing the title feed, the scanner also stopped treating the byte `0x9D` as a C1 control: it is the middle byte of `❯`, so a prompt drawn with that arrow used to swallow every title and status report after it.
  — 浏览器客户端重新说对协议版本：0.6.2 的前端仍向要求版本 2 的 daemon 声明版本 1，页面能加载却看不到任何会话。现在发送 2，真不一致时会把双方版本都写出来。修标题链路时也修了扫描器：它把字节 `0x9D` 当成 C1 控制符，而 `0x9D` 正是 `❯` 的中间字节，于是带这个提示符的终端会吞掉之后所有标题和状态报告。
- **A shell started by the desktop has a UTF-8 locale.** A window-server launch passes no `LANG`, so zsh came up in the C locale: a Chinese name was echoed as reverse-video bytes and `cd 中文` said the directory did not exist. Sessions now name the user's own UTF-8 locale from the system preference, or `en_US.UTF-8` / `C.UTF-8` when the system has one; an environment that already names a locale — including an explicit `C` — is left alone, and Windows, whose console has no locale, is unchanged.
  — 桌面端启动的 shell 现在有 UTF-8 locale：从窗口服务器启动时环境里没有 `LANG`，zsh 会进 C locale——中文文件名被回显成反色的字节串，`cd 中文` 还会说目录不存在。会话现在按系统偏好选择用户自己的 UTF-8 locale，系统没有时退到 `en_US.UTF-8` / `C.UTF-8`；环境里已经写明 locale（包括显式 `C`）就不覆盖，Windows 控制台没有 locale 概念，行为不变。
- **macOS draws its own traffic lights.** The three dots are AppKit's now, on the same 30px line as the rest of the chrome, so they keep the system's shapes, spacing, inactive-window grey, double-click and window menu; full screen collapses them and the header around them. Windows and Linux keep the controls the view draws, now covered by tests that run on every platform.
  — macOS 改用系统自己的红绿灯：三颗圆点由 AppKit 绘制，与其余 chrome 落在同一条 30px 线上，形状、间距、失焦灰色、双击与窗口菜单都是系统的；全屏时它们和围绕它们的标题区一起收起。Windows / Linux 继续使用界面绘制的按钮，并补上了在三大平台都运行的测试。

## 0.6.2

**Desktop: the window's controls in the platform's corner, and a settings window that floats — 桌面端：窗口按钮回到平台该在的一角，设置改成浮窗**

- **Minimize, maximize and close at the top right on Windows and Linux.** The view used to draw macOS's three dots at the top-left on every platform. Windows and Linux now get Windows 11's flat minimize, maximize or restore, and close at the right end of the card header, the close one turning red under the pointer; macOS keeps its traffic lights. The top-right pane's header keeps the room the buttons take, the maximize button carries the system's snap-layout hit target on Windows 11, and with the sidebar folded away the card title no longer leaves the traffic lights' 58 pixels on platforms that have none.
  — 桌面端窗口按钮按平台分角：Windows / Linux 原先也画在左上角的三颗 macOS 圆点，现在换成右上角 Windows 11 风格的最小化 / 最大化或还原 / 关闭（关闭悬停红底白字），macOS 保持红绿灯。占住窗口右上角的那个窗格，其卡片头给按钮留出位置；最大化按钮在 Windows 11 上仍能唤出贴靠布局；侧栏收起时，卡片标题也不再为不存在的红绿灯空出 58px。
- **The settings window floats, and does not cover the page.** Opening settings used to lay the window's wash over every shell, so the appearance settings could not be tried against what they change. The dialog is now a floating card with no backdrop at all: the shells stay visible behind it, the pointer drags it aside by its title bar (it stays inside the window, a margin from the edges, and remembers where it was put for the session), and its title bar no longer drags the whole window.
  — 设置不再盖住整页：原先打开设置会用窗口 wash 覆盖所有 shell，改外观时看不到实际效果。现在是一块没有任何幕布的浮动卡片——底下的 shell 一直可见，按住标题栏即可拖开（限制在窗口内、离边留 12px，并在本次运行内记住位置），标题栏也不再拖动整个窗口。

## 0.6.1

**Desktop: a shell another window drives is shown at its own size — 桌面端：别人驱动的终端按它自己的尺寸显示**

- **A pushed-over screen is the machine's screen, not a reflow of it.** A tab watching a shell another window drives used to lay the replay out at its own pane's width. A full-screen program writes for one width, moves its cursor against it and erases to its edges, so the same bytes at another width wrapped where the machine's window does not and put every later redraw a row or a column off — the pushed-over pane read as a different screen, with columns out of place and a band of the program's own colour where the narrower screen ended. The pane now takes the size the machine reports for the shell (`Options.ResizeByConn`, fed by the session list and the attach reply) and draws that screen: a pane narrower than the shell shows the left of it, exactly as the machine's window shows all of it, and a shell this window drives still takes this pane's size, at the moment the window claims it. A resize is broadcast to the machine's other clients, so their panes follow it without a click, and a window that takes a shell back re-asserts its own size — the pane that released it then follows that size again, instead of going on drawing the shell at the width it had while it drove it, which is the garbled screen a person saw right after 释放.
  — 修复「推过来的终端样式不对」：观看别的窗口驱动的 shell 时，原先会把重放的字节按本窗格宽度重新排版。全屏程序按一个宽度书写、按它移动光标并擦除到边缘，同样的字节换个宽度就会在该换行的地方不换、之后每次重绘都偏一行或一列，于是推过来的窗格看起来像另一个界面——列错位，窄屏结束处还留一条程序自己的底色。现在窗格采用机器上报的 shell 尺寸（`Options.ResizeByConn`，来源是会话列表与 attach 回复）并绘制那块屏幕：窗格比 shell 窄时显示左侧部分，与机器自己窗口显示全屏的关系一致；本窗口驱动时仍采用本窗格尺寸，在接管的瞬间同步。控制器改尺寸会广播给该机器的其他客户端，无需点击即可跟随；接回控制权的窗口会重新确认自己的尺寸，于是刚「释放」的那一端又跟着机器的尺寸排版，而不是继续按自己驱动时的宽度绘制（那正是释放后立刻花屏的原因）。
- **The column's top bar has one button, and a refresh turns while it waits.** The window no longer draws a new-terminal button beside the sidebar toggle: a column of machines gives each of them its own "+", and a second one at the top only asked which machine was meant. A group's refresh now turns from the click until that machine's list lands (and stops after a few seconds if the machine never answers), so a click that was taken no longer looks like one that did nothing.
  — 侧栏顶栏只留一个按钮：不再画「新建终端」——机器各有自己的「+」，顶上那个还得先猜是哪台机器。机器组的刷新图标在点击后转圈，直到该机器的会话列表回来（机器一直不回则在几秒后停下），点下去有反应，不再像没点中。


## 0.6.0

**Desktop & relay: close means close, a diagnostic log, and logs on the relay — 桌面端与中继：关闭即关闭、诊断日志与中继日志页**

- **Closing a tab ends its session.** The card's × and ⌘W now do what closing a tab does in a terminal: the pane and the shell behind it go together. In a split the × used to take the pane out of the layout and leave both the tab and the process running, which contradicted its own 关闭标签 tooltip. A shell that has already ended, or a tab still waiting for its id, is only a view to remove.
  — 关闭 tab 现在等于结束会话：卡片右上角 × 与 ⌘W 会同时关掉窗格和它背后的 shell，也就是普通终端里关闭标签的行为。分屏时原先只是把窗格移出布局，tab 和进程都还在，与「关闭标签」的提示自相矛盾；已经退出的 shell、仍在等待会话 id 的 tab 则只关闭视图。
- **The session panel lists every live shell on this machine.** It used to list only this window's open tabs, so a Mac showed two shells locally while Windows (and the browser) listed all seven still running. The local roster now lists every live session too — including shells started from a browser or phone — and picking a row with no tab opens one. The panel counts sessions, not tabs.
  — 「会话」面板现在列出本机全部运行中的 shell。原先只列本窗口打开的 tab，导致 Mac 本地只有两行，而 Windows 和浏览器看到七行。现在本机也会列出浏览器/手机启动的、没有 tab 的会话，点开即重新打开标签；同一台机器在任何客户端上数量一致。面板统计的是会话，不是标签。
- **The relay admin panel is laid out like the desktop app.** Sign-in/setup is one floating card on the wash; the dashboard is a 212px rail with a floating content card, grouped inset rows, system light/dark following, and a manual appearance toggle. Machines, access, connect, account and logs each have their own page instead of one long column of cards.
  — 中继管理页按桌面端的设计语言重做：登录/初始化是一张落在 wash 上的浮动卡片；仪表盘改为 212px 侧栏加浮动内容卡，分组内嵌行、跟随系统明暗并带手动切换。机器、访问、连接、账户与日志各占一页，不再是一长列卡片。
- **A diagnostic log that answers "why is this pane blank?"** Both `tfd` and `tfdesktop` write a file to the settings directory (rotated at 8 MB, one previous file), with `-log-level` and `-log-dir` to tune it; the desktop's 设置 → 诊断 page shows the path, copies it, reveals it and saves a copy, and the macOS menu has 诊断日志…. Every attach, replay, role, exit and dropped frame carries machine= and session= fields, so one timeline can be matched across the window and the daemon.
  — 新增诊断日志：`tfd` 与 `tfdesktop` 会把日志写到设置目录（8 MB 轮转、保留一份旧文件），可用 `-log-level` / `-log-dir` 调整；桌面端「设置 → 诊断」可查看路径、复制、在文件管理器中显示、另存一份，macOS 菜单也有「诊断日志…」。每次 attach、重放、role、exit 与丢帧都带 machine= / session= 字段，窗口与 daemon 的时间线可以对上。
- **Blank panes fixed at their source.** A closed tab's session object is no longer left registered in the link, so reopening the same shell paints the hub's replay instead of routing it to a terminal nobody draws; a frame for a session nobody watches is counted and said once instead of disappearing; a session row on an offline machine answers with a sentence rather than opening a white tab; and a terminal view that fails to start now says so on screen instead of leaving an empty rectangle.
  — 修复白屏根因：关闭的 tab 不再把旧 session 对象留在 link 里，重新打开同一 shell 时重放会画到新终端而不是旧连接；无人观看的帧会计数并提示一次而不是静默丢失；离线机器上的会话行会给出提示而不是打开白屏；终端视图创建失败也会在界面上说明，而不是留下一个空白矩形。

- **Machines push their diagnostic tail to the relay.** `tfd` and `tfdesktop` post the newest 2,000 lines every 30 seconds, authenticated with the device credential; the relay keeps the latest bounded report per machine, and the panel's `Logs` page reads, downloads or forgets it. A blank pane on a headless Mac can now be diagnosed from another computer.
  — 机器会把自己的诊断日志尾部上报到中继：`tfd` 与 `tfdesktop` 每 30 秒推送最新 2,000 行，使用设备凭据认证；中继按机器保留最新一份有上限的报告，管理页的「日志」页可查看、下载或清除。无头 Mac 上的空白窗格因此可以在另一台电脑上排查。

## 0.5.5

**Desktop: a split stays with its tab — 桌面端：分屏跟随自己的 tab**

- **Picking another tab no longer rearranges the split.** Each split's panes now share a workspace that keeps their layout, their focused pane and their zoom while another tab is on screen. Picking a tab of its own puts the workspace away; picking one of its panes brings the same layout back. The sidebar keeps the workspace's row and its terminals in place through the switch instead of folding one tab into another.
  — 切换其他 tab 不再改写分屏：分屏的窗格现在属于同一个 workspace，保存布局、焦点窗格与缩放；选到独立 tab 时把布局收起，再点回其中任一窗格就原样恢复。侧边栏在整个切换过程中保持 workspace 行与其终端不变，不会再把一个 tab 折进另一个里。
- **⌘T opens a tab of its own.** The new shell no longer takes the focused pane's place; the split stays in the sidebar, put away, and a new remote session is shown once it mounts.
  — ⌘T 现在新开一个独立 tab，不再占用当前窗格的位置；分屏留在侧边栏里（收起），远程新会话挂载后会直接显示。
- **Closing panes and tabs edits the right layout.** A pane's tab now comes out of its workspace whether that workspace is on screen or put away; a workspace left with one terminal settles back into an ordinary tab.
  — 关闭窗格或 tab 会从正确的 layout 中移除：无论 workspace 正在显示还是已收起；只剩一个终端时自动回到普通单终端 tab。

## 0.5.4

**Desktop: one tab for a split's terminals, and a settings sheet in the platform's style — 桌面端：分屏归为一个多终端 tab，设置页改为平台样式**

- **A split is one tab, with its terminals listed under it.** The split's row stays one tab, and the panes after the first hang beneath it on a dotted branch; clicking one focuses that pane. A pane whose shell title is only the hostname takes the directory as its name, so four panes no longer read as four copies of the machine.
  — 分屏仍只占一个 tab 行，第一个窗格就是父行，其余窗格以点状树线挂在下面，点子行即可聚焦对应终端。shell 标题只是主机名时改用目录名，四个窗格不会再显示成四台同名机器。
- **A tab's state and its close stay on the row at rest.** The clock and the default checkmark are gone. Working is the loading ring, needs-you an amber dot, finished-unseen a green dot, failed a red dot, an exited shell a quiet ring, and a pane at its prompt a hollow ring. The close sits at the right of every tab row without waiting for a hover.
  — tab 行不再显示时间，也不再默认画勾选：working 用旋转 loading 环、需要你为琥珀色圆点、完成未看为绿色圆点、失败为红色圆点、已退出为空心环、空闲窗格为空心圈；close 常驻在每行右侧，不再依赖悬停。
- **Each machine keeps refresh and new-terminal on its group.** Both buttons stay visible at rest rather than appearing under the pointer; refresh asks that machine for its session list again.
  — 每个机器分组的刷新与新建终端按钮都常驻，不再只在悬停时出现；刷新会重新拉取该机器的会话列表。
- **The split divider follows the pointer by the room it divides.** A short drag no longer jumps the split to its 12/88 limit because the drag was measured against the divider's own eight pixels instead of the branch's width.
  — 修复分屏分隔条拖动：原来按分隔条自身 8px 宽度换算比例，轻轻一拖就跳到 12/88 的边界；现在按整个分屏区域的尺寸换算，拖动跟手。
- **Settings is a sheet on the window's wash.** The dimmed modal and its grey scrim are gone: the wash covers the window and the settings card floats on it. Each page has a fixed header, the form pages sit on the soft group panel the other pages use, controls line up on the right, and the saved line is a quiet footer with a state dot. Inputs, segmented controls and the size pop-up take the platform's light and dark field style.
  — 设置改为落在窗口 wash 上的面板：去掉压暗遮罩和灰色背景；每页有固定页头，表单页放进与其他页一致的浅灰分组面板，控件右对齐，底部保存状态改为带状态圆点的静音 footer；输入框、分段控件和字号 popup 使用平台风格的浅色/深色字段样式。

## 0.5.3

**Desktop: a folding sidebar, per-machine refresh, and a takeover panel at the foot — 桌面端：侧栏折叠、按机器刷新、接管浮窗移到底部**

- **The sidebar folds instead of vanishing.** One progress now drives the column's width, the cards' left edge and the header's inset over 260ms, while the rows slide out under a clip. The top bar's buttons keep their place over the cards and hand the corner to the card's header, so the drawer no longer jumps from open to gone in one frame.
  — 侧栏改为真正的折叠动画：一个进度同时驱动栏宽、卡片左缘和卡片头缩进，260ms 内一起移动；会话行在裁剪下滑出，顶栏按钮先留在窗口原位，再把位置交给卡片头，不再一帧之内从全开跳到全隐。
- **Each machine's group refreshes its own session list.** A refresh button sits at the right of a group header, left of the new-terminal button, and asks that machine for its sessions again; a tooltip names the action without borrowing the window-wide refresh shortcut.
  — 每个机器分组右侧新增刷新按钮，位于「新建终端」左边，只重新拉取该机器的会话列表；提示文案单独一条，不冒充全局刷新的快捷键。
- **The takeover panel moved to the bottom center of the pane.** The panel that names who holds a shell, explains the read-only state and offers 拿回控制权 / 接管 is now a floating card anchored above the pane's foot rather than a block in its top-left corner, in the same surface and buttons as the rest of the window. It still closes on an outside press or Escape.
  — 「谁在控制这个终端」的接管浮窗从窗格左上角移到窗格底部居中，改为与应用一致的浮动卡片（同样的圆角、阴影与按钮），内容仍是持有者、只读说明和拿回/接管按钮；点击外部或 Esc 仍可关闭。
- **Window controls are macOS traffic lights, drawn on every platform.** Close, minimize and zoom are three dots in the top-left corner with their glyphs on hover; the system's window buttons are hidden everywhere, and the top-right card header no longer keeps a corner clear for them.
  — 窗口按钮统一为左上角自绘的 macOS 风格红绿灯：关闭、最小化、缩放，悬停时显示字形；所有平台都隐藏系统按钮，右上角卡片头也不再为它们预留空位。

## 0.5.2

**Desktop: the window draws its own controls, and a taken shell gets a strip of its own — 桌面端自绘窗口按钮，接管提示单独成条**

- **Windows and Linux draw their own minimize, maximize and close.** The system's buttons used to sit over the top-right card's header, on top of the chips and the takeover controls; the view could not tell where they were, and a sentence that ran under them was half readable. The window now hides them (`HideWindowButtons`) and draws the three itself in the card header's corner, as kero does: quiet greys, a danger wash under close, restore while maximized, and Windows 11's snap layouts over the maximize control. macOS keeps its traffic lights.
  — Windows 与 Linux 不再使用系统按钮：它们原先压在卡片右上角，正好盖住卡片头里的 chip 和接管控件，界面也无从为它们让位，句子压在按钮下面就只读得到一半。现在隐藏系统按钮，由界面在卡片头的同一角自绘最小化、最大化/还原与关闭（对齐 kero）：常态灰色、关闭悬停转危险色、最大化后显示还原图标，最大化按钮悬停仍有 Windows 11 的贴靠布局；macOS 继续用原生红绿灯。
- **The takeover sentence is a strip under the header, not a chip squeezed into it.** A shell another client holds (or nobody holds), and a shell that has ended, now say so on a full-width strip between the header and the terminal: state dot, the sentence, and the 接管 / 拿回控制权 / 关闭标签 button at its right. The strip is below the window controls, so it cannot be covered by them; the floating 只读 badge that duplicated it is gone, and a split's other panes still mark themselves 只读 or 已退出 in their own headers. A window with no shell open keeps a header too, so it still has a close button.
  — 接管提示改为卡片头下方、终端上方的一条整宽状态条：状态点、完整句子，以及右侧的「接管 / 拿回控制权 / 关闭标签」按钮；它位于窗口按钮下方，因此不会再被盖住。原先重复的右上角浮动「只读」徽标与卡片头里的 chip 都已移除；分屏中未聚焦的窗格仍在自己头部标注「只读 / 已退出」。窄窗口里句子不再被挤成省略号。没有打开终端时卡片也保留头部，窗口仍然有「关闭」可用。
- **A taken pane is no longer half a pane.** The glow along a pane another client drives was wrapped in a `Fill` box that laid out beside the card, so the card took half the body and the wash showed below it whenever the light was on. The glow is an overlay now, and the pane keeps its height.
  — 修复「本机 shell 被其他客户端接管后，卡片只剩半高、下方露出 wash」的缺陷：边缘光效的容器原先是与卡片并列的 Fill 元素，抢走了一半高度；现在光效是真正的覆盖层。

## 0.5.1

**Every platform records the same shortcut — 三个平台记下的快捷键一致**

- **⌘ and Ctrl are one key on Windows and Linux.** Recording a shortcut there wrote "cmd+ctrl+shift+t": the platform's shortcut modifier *is* Control, so the two names were the same bit spelled twice. It is written once now, in the settings file and in the menus, and a test runs the round trip with the Command key absent so the case cannot come back.
  — 在 Windows/Linux 上，平台的快捷键修饰键本身就是 Control，所以录制时曾写成 "cmd+ctrl+shift+t"——同一个按键写了两遍。现在只写一次，设置文件与菜单一致；并新增一个「没有 Command 键」的往返测试，防止这个情形回归。
- **The tests read the app's own strings, and ignore a checkout's line endings.** The notification tests compared against Chinese literals and failed on a machine whose locale is English; the token test compared the generated sheet byte for byte and failed on a Windows checkout that turns LF into CRLF. Both compare against the source now.
  — 通知相关的测试原先写死中文文案，在英文 locale 的机器上会失败；token 一致性测试按字节比较，在会把 LF 换成 CRLF 的 Windows 检出上会失败。两处都改为与源数据比较。

## 0.5.0

**Desktop & web: kero v2's look — 桌面与浏览器换成 Kero v2 的外观**

- **One palette, two clients.** `internal/uitokens` now holds the chrome's colors, the five window washes and the terminal themes, and `web/src/tokens.css` is generated from it — a test fails when the two disagree, so the desktop and the browser cannot drift apart again. Both appearances are real: 设置 → 外观 offers 跟随系统 / 白天 / 暗黑 and five backgrounds (粉紫、雾蓝、草绿、暮色、纯色), and the terminal's own colors follow the card they sit on.
  — 新增 `internal/uitokens`，统一保存桌面与浏览器共用的颜色、五种窗口背景与终端配色；`web/src/tokens.css` 由它生成，并有测试保证两者不再漂移。浅色与深色这次都是真的：「设置 → 外观」可选跟随系统/白天/暗黑与五种背景（粉紫、雾蓝、草绿、暮色、纯色），终端配色也跟着卡片走。
- **The desktop's frame is kero's.** A wash behind floating cards, a 268px sidebar grouped by machine with session rows that carry a state glyph and a relative time, a 44px card header with the shell's name, its directory, its message chip and hover controls, and an 8px gap between split panes. The band across the top and the status bar are gone: the fleet's state lives in the sidebar's foot, and the window's title carries the count of what needs you.
  — 桌面窗体改为 Kero 的结构：渐变 wash 上浮着卡片；268px 侧栏按机器分组，会话行带状态图标与相对时间；44px 卡片头显示 shell 名、目录、消息 chip 与悬停控件；分屏之间留 8px 缝。原先的顶栏与状态栏取消，舰队状态收进侧栏脚，窗口标题显示「需要你」的数量。
- **What a shell is doing.** The daemon reads a program's own reports (OSC 7501), ConEmu's notifications (OSC 9), the bell, a spinner in the terminal title and the arrival of output, and reports `idle / working / waiting / error / exited` with a message, a kind, a progress and an unread mark on the wire. `⌘J` jumps to the next session that needs you, the sidebar's foot counts 需要你 / 运行中 / 已完成 and walks them on a click, and an unread row's name is set heavier.
  — daemon 现在读懂终端里的信号：程序自己上报的 OSC 7501、ConEmu 的 OSC 9 通知、响铃、标题里的转圈字符，以及输出是否还在流动，据此在协议里给出 `idle / working / waiting / error / exited`，附带消息、类型、进度与未读标记。`⌘J` 跳到下一个需要你的会话，侧栏脚统计「需要你 / 运行中 / 已完成」并可点击逐个跳转，未读行的名字加粗。
- **Splits are panes, not tabs.** `⌘D` splits the shell on screen and the sidebar keeps one row for the workspace, with the pane count in its tooltip and the most urgent of its panes' states on the row; closing one pane takes it out of the split and gives its shell a row of its own again, still running. The keys are kero's: `⌘]` / `⌘[` walk the panes, `⌘⌥←→↑↓` cross the dividers, `⌘⇧↩` fills the body with the focused pane, `⌘R` names the shell on screen (refreshing moved to `F5`), and `⌘=`, `⌘-`, `⌘0` size the terminal.
  — 分屏现在是「窗格」而不是「标签页」：`⌘D` 分屏后侧栏只保留一行（提示里写着窗格数，行上显示最紧急的那个窗格的状态），关掉一个窗格只是把它从分屏里取出，那个 shell 仍在运行、并重新获得自己的一行。快捷键对齐 kero：`⌘]` / `⌘[` 在窗格间移动，`⌘⌥←→↑↓` 跨分隔线换窗格，`⌘⇧↩` 让当前窗格占满整个窗口，`⌘R` 重命名当前 shell（刷新改为 `F5`），`⌘=`、`⌘-`、`⌘0` 调整终端字号。
- **A command palette on `⌘P`.** Sessions, machines and commands in one list, with kero's own fuzzy order and each command's keys beside it.
  — `⌘P` 打开命令面板：会话、机器与命令同列，模糊排序沿用 Kero 的算法，每条命令右侧标出快捷键。
- **Settings is a window again.** Pages down the left, grouped panels on a card, and a real appearance page; the terminal pane keeps its font picker and size.
  — 设置窗口重做：左侧页列表、右侧卡片上的分组面板，并新增「外观」页；终端页保留字体与字号选择。
- **The browser client has no tab strip.** The session list is the switcher — click a row, the pane attaches; the composer, the on-screen keys, the inspector, the read-only banner and the phone drawer stay. Rows carry the same five state glyphs, and the card sits on the same wash.
  — 浏览器端删除标签栏，改为「侧栏即会话列表」：点一行就附着一个会话；输入框、触屏按键、检查器、只读横幅与手机抽屉都保留。会话行使用同一套五态图标，卡片坐在同一片 wash 上。
- **Native notifications, and a Dock badge.** Mygo is upgraded to the version that has them. A session that comes to need you, a program that reports a failure, or an agent that finishes while the window is not in front now posts a notification whose click puts that session on screen; the Dock icon counts what needs you. 设置 → 通知 turns each one off on its own, and 恢复默认 puts them all back.
  — 升级 mygo 到带通知 API 的版本：会话变成「需要你」、程序报告失败、或 agent 在窗口不在前台时完成，都会发系统通知，点通知直接切到那个会话；Dock 图标显示需要你的数量。「设置 → 通知」可以分别关掉三种通知，「恢复默认」恢复全开。
- **The program in the pane.** The daemon now reads the foreground process group of a terminal — `tcgetpgrp`, then the process's name from `/proc` on Linux or `kern.proc.pid` on macOS — and reports it on the wire. Both clients show it as a chip on the card's header when it is a program rather than the shell: `claude`, `npm`, `vim`, without the program having to report anything.
  — daemon 会读取终端的前台进程组（`tcgetpgrp`，再取 `/proc`（Linux）或 `kern.proc.pid`（macOS）里的进程名），并放进协议；两端在卡片头上把它显示成一个 chip——`claude`、`npm`、`vim`，程序自己什么都不用上报。
- **Every shortcut is rebindable.** The window's keys moved into one action registry: the keyboard, the command palette, the buttons on the empty card and the menu accelerators all read it, so a key rebound in 设置 → 键盘 is the key the window answers to. The page records the next keys typed, names a collision with the action that already holds them, clears a shortcut with ⌫, and 恢复默认 puts the defaults back.
  — 快捷键收进一张 action 表：键盘、命令面板、空卡片上的按钮、菜单加速键都读它，所以在「设置 → 键盘」里改的键就是窗口实际响应的键。该页会录制下一次按下的组合、冲突时点名已经占用它的动作、用 ⌫ 清除，「恢复默认」恢复全部默认键。
- **What did not move.** The fleet, the relay, one controller per shell with viewers, byte-offset resume, the desktop's splits and the browser's composer are untouched. The protocol gained the status fields and went to version 2, so a client built before this release must be reloaded once.
  — 舰队、中继、每个 shell 的单控制者租约、按字节偏移续接、桌面分屏与浏览器输入框都没有动。协议新增状态字段并升到版本 2，升级前构建的客户端需要刷新一次。

## 0.4.22

**Desktop & agent: a session-id data race closed — 修复会话 ID 的数据竞争**

- **The window no longer reads a session's id unlocked.** The link's reader goroutine gives a session its id when the hub names it, and the UI thread read that id while it drew — the race the detector caught the moment a window was drawn with a `created` frame in flight, in the path that asks for a shell's lease. The id now goes through a locked accessor, as the role and the epoch already did.
  — 桌面端读取会话 ID 时不再绕过锁：链路的读取 goroutine 在 hub 命名会话时写入 ID，而 UI 线程在绘制时读取它（抢锁请求就走这条路）。现在 ID 通过带锁的访问器读取，与角色、租约纪元一致。
- **The tests that hid it are honest now.** A test's shell runs `sh` where there is one and `cmd.exe` on Windows; a temporary working directory is used only where the platform can be asked where a process stands, because a ConPTY process holds its directory; and the follow test waits for where the process stands rather than for the echo of the line that moves it.
  — 顺带修掉掩盖问题的测试：Windows 上用 `cmd.exe`；只在能查询进程目录的平台上使用临时目录；继承测试等待进程真正切换目录，而不是命令回显。

## 0.4.21

**Desktop: split panes with ⌘D / ⌘⇧D, and every new shell opens where the shell beside it stands — 桌面端分栏终端与工作目录继承**

- **Split panes.** `⌘D` splits the shell on screen beside a fresh one and `⌘⇧D` above or below it, each with its own sheet and a divider the pointer drags (the arrows move it once it has the focus). The new shell is an ordinary shell: the 会话 panel lists it, and closing its pane leaves it running. The pane on screen is the one the composer types into, the header describes and `⌘W` closes. Splitting a shell on another machine opens the second shell there, over the same relay link.
  — 桌面端支持 `⌘D` 左右分栏、`⌘⇧D` 上下分栏，中间分隔线可拖动（获得焦点后方向键也可移动）。新分栏是普通标签页：出现在标签栏与会话列表中，关闭分栏不会结束进程。聚焦分栏即当前终端：输入框、顶栏、状态栏都跟随它，点击或标签栏可切换焦点；有分栏时 `⌘W` 关闭当前分栏，无分栏时关闭标签页。对远端机器的终端分栏会在同一台中继链路上再开一个shell。
- **A new shell opens where the shell beside it stands.** The daemon asks the operating system where a session's process is now — `/proc` on Linux, `lsof` on macOS — and a create that names a session with `from` opens in that directory, so a split or a second tab continues where the first shell is instead of at the home directory. The directory the shell was started in is the fallback where the platform cannot say, and a follow of a session that is gone keeps the directory the client asked for. Both clients send it: the desktop on `⌘T`, the sidebar's ＋ and every split; the browser on `⌘T` and its 新建终端.
  — 新终端继承相邻终端的当前工作目录：daemon 通过 `/proc`（Linux）或 `lsof`（macOS）查询会话进程的实时 cwd，create 帧带 `from` 指定继承来源，分栏和新标签页都会落在前一个 shell 当前所在目录，而不是每次都回到 home。平台无法查询时回退到 shell 启动目录，被继承的会话已不存在时回退到客户端指定目录。桌面端（`⌘T`、侧栏 ＋、每次分栏）与浏览器端（`⌘T`、新建终端）均已支持。

**Desktop & agent: name a shell, and an interface that speaks English — 会话重命名与中英文切换**

- **Name a shell.** A `rename` frame sets a session's title, which every client's tab and row reads; an empty title clears it and the shell goes back to being named after its program. The lease is not consulted — a title is a label on a shell everybody watching sees, not the keyboard. In the desktop a double-click on the shell's row in the 会话 panel puts its name into a field (Enter commits, Escape lets go), with the same act in the row's menu; in the browser a double-click on the tab does the same.
  — 新增 `rename` 帧设置会话标题，所有客户端的标签页与会话行都会读取；清空标题则恢复为按程序名命名。租约不参与此事：标题是给大家看的标签，不是键盘。桌面端在「会话」面板双击某一行的名字即可改名（回车提交、Esc 取消），行菜单里也有同样一项；浏览器端双击标签页同理。
- **The rename field reads as the row's own.** The field sits inside the row at the label's size, on the panel's raised field ground, its hairline border turning accent while the caret is in it — the name becoming editable is the row changing state, not a box the row's height landing on it. A press anywhere else commits the name, the way a blur does in the browser client, and a field nobody typed into names nothing: the shell keeps the name it had instead of having the name it showed pinned to it as a title. In the browser the tab's field takes the pill's own inset with the design system's focus ring, and grows to the room the label had.
  — 桌面端改名输入框改为行内编辑样式：与行高相衬的紧凑高度、与标签同款字号、面板的浮起字段底色，边框在获得焦点（即出现的瞬间）时转为强调色；点击别处即提交，与浏览器端失焦提交一致；未改动的字段不会把当前显示的名字钉成标题，shell 保持原有命名。浏览器端标签页输入框改为适配胶囊内距、带焦点环，并占满标签原有宽度。
- **The interface speaks English.** The desktop's chrome is now a mygo i18n catalog with Chinese and English, and 设置 → 终端 carries a 语言 field: 跟随系统, 中文 or English, applied on the click and kept for the next launch. The browser client is unchanged and stays Chinese.
  — 桌面端界面全面接入 mygo i18n，提供中文与英文两套文案，「设置 → 终端」新增「语言」选项：跟随系统、中文、English，点击即生效并记住下次启动。浏览器端暂不涉及，仍为中文。
- **The settings window's panes line up.** Every explanatory line now starts in the control column instead of at the pane's edge, so the notes, the status lines and the fields read as one grid.
  — 设置窗口各面板的说明文字统一对齐到控件列，不再从面板左边缘开始，说明、状态行与输入框成为一个整体。

## 0.4.20

**Desktop & agent: a tab whose relay link dropped renders the shell again — 远端标签页断线重连后自动恢复渲染**

- **Remote tabs re-attach after a link drop.** A tab watching another machine's shell was bound to the relay stream it was opened on; when that stream ended — a relay restart, a member flap, or a slow viewer evicted under load — the tab stayed on screen forever showing nothing new, and clicking its row only selected the dead pane. The window now re-watches such a tab on the machine's link as it now stands: the click finds a live pane, and looking at the machine again (picking it in the 机器 panel, refreshing its sessions) heals its tabs without a click, the way the browser client's reconnect already re-attached its own. The hub replays the session's ring from byte 0 onto a fresh terminal, so the shell's screen is painted again rather than laid over the stale one.
  — 彻底修复查看其他机器终端时，中继流断开（中继重启、机器掉线、高负载下慢速查看者被踢出）后标签页卡住、什么都不渲染且再次点击也无效的缺陷。重新点击该会话或再次查看该机器时，窗口会自动在新链路上重新附着该终端，Hub 从字节 0 重放会话环形缓冲区并绘制到全新终端，立即可见 Shell 界面。
- **A busy session no longer kills its remote viewer mid-replay.** The relay agent's outbound queue is shared by every viewer of a machine; when it filled under sustained output, the frame in flight was dropped and the error was read by the hub as a dead client — which detached the viewer before its replay had landed and left exactly the blank pane above. A full queue now applies bounded backpressure (the hub's own write deadline bounds the wait) instead of silently evicting the viewer.
  — 修复 Mac 等高输出机器在中继链路拥塞时，远程查看者的附着回放帧被静默丢弃、导致 Hub 直接判定该查看者掉线的问题。出站队列满时现在按 Hub 写超时做有界背压，而非静默踢掉查看者造成白屏。

## 0.4.19

**Desktop: refined sidebar spacing, removed redundant section hairlines, and snug machine panel height — 侧边栏布局间距深度精简与优化**

- **Eliminated intrusive section header dividers.** Removed redundant bottom hairline borders from section title rows (`会话` and `机器`), allowing headers to flow naturally into list items without dividing them with harsh lines or sandwiching the machine title between double borders.
  — 移除了「会话」与「机器」面板标题栏下方冗余的分割线，让标题与下方项目自然衔接，彻底消除「机器」标题被上下两道分割线夹在中间的生硬感。
- **Compact header & row heights.** Standardized section header height to 30px with 24px icon buttons, session rows to 34px, and machine rows to 32px with 8px horizontal padding, bringing tight, balanced vertical rhythm matching modern desktop apps.
  — 将分类标题栏高度优化为 30px、操作图标调整为 24px；会话行高精简至 34px、机器列表行高精简至 32px，左右外边距统一定为 8px，视觉重心更加规整平衡。
- **Dynamic machine panel content fitting.** Calculated exact content height for the machines panel (`headH + itemsH + padding`), eliminating awkward blank dead space below the machine list before the bottom divider and "设置" button.
  — 机器面板高度改为基于名册行数精确计算内容高度，彻底解决机器列表下方与底部分割线及「设置」按钮之间大块多余空白的问题。
- **Streamlined settings button.** Reduced bottom settings toolbar height to 36px with 12px padding, ensuring the sidebar footer feels compact, lightweight, and cohesive.
  — 底部「设置」按钮高度调整为更精致的 36px，使侧边栏整体在深色主题下更加紧凑耐看、高级舒适。

## 0.4.18

**Desktop & Web: manual refresh controls for sessions and fleet machines, shortcut support — 会话与机器列表手动刷新功能**

- **Dedicated refresh buttons in panel headers.** Added dedicated refresh icon buttons (`↻`) to both the "会话" (Sessions) and "机器" (Machines) panel headers in the desktop app sidebar. Users can instantly poll the relay and target hubs for the latest live sessions and roster without needing to restart the app or deselect machines.
  — 桌面端侧边栏「会话」与「机器」面板标题栏新增独立刷新图标按钮（`↻`）。用户可随时主动拉取目标机器与中继集群的最新会话列表及机器在线状态，无需重启应用或反复切换选项。
- **Global refresh shortcuts.** Supported `⌘R` (macOS) / `Ctrl+R` (Windows) and `F5` shortcuts across the desktop client to trigger a unified refresh of all machines and active machine sessions, with clean, non-intrusive status feedback in the bottom status bar (`已刷新终端与机器状态`).
  — 全局支持 `⌘R` / `Ctrl+R` 与 `F5` 快捷键，一键同时刷新机器集群列表与当前聚焦机器的全部会话，并在底部状态栏静默显示刷新提示。
- **Relay snapshot on-demand query.** Extended the relay protocol and client transport to support explicit client-initiated `tunnel.TMachines` queries, allowing instant retrieval of the up-to-date fleet snapshot anytime a refresh is requested.
  — 扩展中继协议与客户端传输层，支持客户端随时主动请求 `tunnel.TMachines` 快照帧，确保手动刷新时立即获取最新的机器名册与健康状态。
- **Web client sidebar refresh.** Added an in-app refresh button to the Web UI sidebar header, enabling instant resync of all connected fleet hubs and relay machines from mobile and desktop browsers alike.
  — Web 客户端侧边栏同步新增刷新按钮，手机与网页端用户也可一键刷新所有机器及其会话。

## 0.4.17

**Desktop: pending tab states, stream reconnect resilience, and sidebar session retention — 终端连接状态呈现与会话持久性修复**

- **Distinguish pending creation from exited state.** Fixed a bug where brand new pending tabs (`tb.id == ""`) were falsely rendered as "已退出" with an error banner "会话已退出，最后几行仍可读取。 [关闭标签]". Pending tabs now properly display neutral "正在创建终端…" / "创建中…" and will never prematurely display exit warnings or badges while awaiting hub creation.
  — 彻底修复新建终端标签页等待分配会话时被误判为“已退出”并弹出关闭警告的问题。等待连接的标签页现在正确显示“正在创建终端…”与“创建中…”，不再误报退出。
- **Preserve live remote sessions in the sidebar.** Fixed an issue where closing or dismissing a tab watching a remote session caused the remote session to be permanently hidden from the sidebar session list ("会话 0"). Live remote sessions remain listed in the sidebar at all times so they can always be re-opened.
  — 修复关闭远端标签页导致侧边栏远端会话被永久隐藏、显示为“会话 0”的严重缺陷。远端机器上实际运行的会话将始终保留在侧边栏列表中，随时可重新点击查看。
- **Resilient relay stream recycling and tab retention.** Fixed stream lifecycle management in `relayclient` so closed or dropped streams are properly forgotten rather than reused as dead zombie streams. Link disconnections now preserve existing terminal tabs and buffer history rather than automatically deleting them.
  — 修复中继客户端在网络闪断或流结束时错误复用已关闭死流的问题，且网络连接断开时不再强制关闭和销毁终端标签页，保留终端缓冲区历史。
- **Deduplicated PTY resize synchronization.** Optimized `syncSize` to track synced dimensions and eliminate redundant `CTResize` frames during tab selection when terminal dimensions have not changed.
  — 优化终端尺寸同步逻辑，记录上次同步尺寸，避免重复点击标签页时向远端终端发送冗余 resize 帧。

## 0.4.16

**Desktop: terminal PTY size sync, remote machine tab creation, unclipped header buttons — 终端尺寸同步与远端新建标签修复**

- **Remote PTY size synchronization.** Fixed broken terminal rendering, repeating prompts, and duplicated progress lines on remote sessions (e.g. Windows ConPTY). Terminal dimensions are now recorded while viewing and immediately synchronized to the remote Hub upon lease takeover (`接管`), ensuring remote PTY columns always match the desktop window width.
  — 彻底修复远端终端（如 Windows ConPTY）文字重叠、进度条重复刷屏和换行错位的问题。终端视图尺寸在只读时记录并在接管控制权时自动同步给远端 Hub。
- **Creating new tabs on selected remote machine.** Fixed an issue where clicking `+` in the session panel or pressing `⌘T` while viewing a remote machine would mistakenly create a local tab if an existing local tab was active. New tabs now always target the selected remote machine.
  — 修复切换到远端机器后点击新建终端仍创建本地终端的问题。新建终端将始终基于当前选中的目标机器创建。
- **Unclipped header release button.** Replaced the shrinkable header action button with a fixed-size pill button (`Shrink(0)`), ensuring "释放控制权" is never truncated into a single character ("释").
  — 修复顶栏操作按钮在边距收紧时被挤压成单个文字“释”的问题，采用固定尺寸药丸按钮保持完整文案。

## 0.4.15

**Desktop: bottom lease controls, remote new tab fix, Windows caption padding — 底部控制权操作与多端交互优化**

- **Bottom lease status and actions.** Live lease status (只读 / 控制中) and action
  buttons (接管 / 释放控制权) are now displayed in the bottom status bar, safe from
  window titlebar button overlap and reachable at all times.
  — 底部状态栏增加只读/控制中状态指示及“接管”、“释放控制权”按钮，随时可触达。
- **Windows titlebar caption padding.** The top header now applies right padding on
  Windows to prevent header controls from overlapping or being covered by native
  Windows minimize/maximize/close buttons.
  — Windows 标题栏增加右侧避让边距，避免顶栏操作按钮与原生关闭按钮重叠。
- **New tab targets the active remote machine.** Creating a new terminal tab (via
  sidebar or `⌘T`) while controlling a remote machine now opens the tab on that
  remote machine rather than defaulting to local.
  — 修复控制其他机器时新建终端的问题，快捷键与新建按钮将跟随当前控制的机器。
- **Strictly manual remote takeover.** Opening an existing remote session no longer
  auto-claims the lease. It opens as read-only, requiring the user to explicitly
  click "接管".
  — 彻底移除远端会话打开时的自动接管，打开后保持只读，接管与释放均改为手动。

## 0.4.14

**Desktop: macOS native dark styling, squircle icon, manual lease control — 桌面端深色外观与交互细节优化**

- **macOS dark styling and clean title bar.** Fix the 1px white line at the top of the
  macOS window by enforcing native dark appearance and using a hidden titlebar
  style. App icon now uses an Apple-compliant squircle to eliminate the white Dock
  background platter.
  — 适配 macOS 原生深色外观，消除顶部白线，规范图标底板避免 Dock 显示白底。
- **Cleaner minimal navigation.** Remove the redundant top-left logo block and
  the intrusive header "新建终端" button. New sessions can be created via the
  sidebar button or `⌘T`. The header supports native window dragging and traffic
  light spacing when the sidebar is collapsed.
  — 精简侧栏顶部标识与顶部新建按钮，支持窗口拖拽及侧栏折叠避让红绿灯。
- **Clear machine status.** Remote online machines now display a green status dot
  instead of grey, preventing confusion about connectivity.
  — 远端在线机器状态指示灯调整为绿色，状态更直观。
- **Human-readable terminal names & recycled numbering.** Terminals now show clean
  names like `zsh 1`, `zsh 2` or `终端 1`, `终端 2` instead of raw internal backend
  IDs (`s1`, `s3`). Closing and opening tabs recycles unused ordinal numbers.
  — 终端名称优化为友好的程序名和序号（如 `zsh 1` 或 `终端 1`），消除裸露的后端内部 ID（`s1`、`s3`），关闭会话后可回收序号。
- **Clearer lease and takeover terminology.** Add self-explanatory tooltips and helper
  descriptions for read-only mode, takeover, and release actions, clarifying multi-client
  keyboard input vs view-only observation.
  — 补充多端协同模式提示，优化只读模式、接管输入及释放控制权的文案与悬浮提示。
- **Explicit, manual lease takeover and release.** Switching to another machine
  no longer automatically claims its shell or drops existing leases. Taking over
  and releasing control are now strictly manual.
  — 切换机器或会话不再自动接管或释放控制权，接管与释放均改为手动操作。

## 0.4.13

**Desktop: a quieter terminal, with its colors intact — 桌面终端界面重构**

- **Neutral black and gray chrome, colorful terminal output.** Remove the violet
  theme accent while preserving the terminal's ANSI palette and TrueColor output.
  The embedded application logo remains visible in the sidebar.
  — 黑灰界面保留彩色终端输出，侧栏显示应用 Logo。
- **More room for the terminal.** Remove the duplicate tab strip, command composer,
  history strip and inspector entry. Type directly in the native Mygo terminal;
  switch sessions in the compact sidebar or with keyboard shortcuts.
  — 移除重复导航和底部命令输入栏，直接在终端内输入。
- **Compact navigation.** Session and machine rows use a single line; long labels
  truncate, and the header and status line stay on one line. `Cmd/Ctrl+B` toggles
  the sidebar and `Cmd/Ctrl+W` closes the current tab.
  — 压缩会话、机器和顶部布局，支持隐藏侧栏及关闭当前标签。
- **Working settings dialog.** Sidebar settings and `Cmd/Ctrl+,` open the same
  Mygo dialog, with Terminal, Remote, Service and Updates sections. Font controls,
  relay credentials and CA, service and control switches, and updates remain
  available. The dialog fits the minimum desktop window size.
  — 设置整合为四分类弹窗，保留字体、远程连接、证书、服务及更新功能。
- **Closed tabs stay closed.** Closing a tab removes its session row without
  ending the process. Local roster refreshes do not reopen dismissed tabs or
  remove remote tabs. Takeover and release behavior is preserved; exited sessions
  keep their final output until explicitly closed.
  — 关闭后不再显示或被刷新重新打开，保留接管、释放及退出前最后输出。

Browser UI remains unchanged in this release. — 本版本仅重构桌面端界面。

## 0.4.12

**The lease's edges: this machine's own shell, a shell nobody holds, a shell that is gone**

- **释放控制权 is not offered on a shell of this machine's.** The button was drawn
  for every shell the window was driving, including the ones running here — and
  on those there is nobody to hand the lease to. The release drops it and the hub
  hands it to no one, so the only thing the button did was stop the window in
  front of the shell from taking the typing of the person sitting at it. It is
  drawn for the shell this window drives *on another machine*, where the lease
  goes back to the machine the shell runs on and that machine can type again. The
  browser client drew it on the same wrong rule and now draws it on the same
  right one.
- **A shell that has ended keeps its tab, and the tab finally knows it.** The hub
  forgets a session the moment its process exits — before the exit frame and
  before the session list — so no summary ever says `alive=false`, and the
  desktop tab, which learned liveness from the attach summary, sat over a dead
  shell offering 结束 for a process that was gone, with no way to close the tab.
  The exit frame is the only place a tab hears it now: 已退出 arrives with its
  exit code in the tooltip, the tab dims with the exit's colour on its dot, its
  row offers 关闭 where a running shell's row offers 结束, and the band offers
  关闭标签 without repeating the sentence. Nothing is sent to a shell that has
  ended — no keystrokes, no resize: a resize answered `no_session` was a red toast
  raised by the act of reading a shell's last lines.
- **The hub's refusals say who holds the shell, or that nobody does.** A frame
  that needs the lease was answered "another client controls s1" whatever the
  state was, including a shell the same client had just let go — a sentence with
  no client to name, which sends the person reading it looking for somebody who is
  not there. The hub now tells a free lease from a lost one: the refusal is
  *nobody holds s1; claim it first* when there is nobody to name, and a frame from
  a client one handoff behind is told the lease has moved rather than that someone
  took it. The browser client's 结束 was the case that mattered: it claimed only
  when somebody else held the lease, so on a released shell the close was refused
  and 结束 did nothing at all. It claims in every case now, stealing only from a
  live holder.
- **A shell nobody holds is marked read-only, in both clients.** A free lease is
  not the same fact as a taken one, but it is the same answer at the keyboard,
  and the desktop drew nothing for it: no mark on the pane, no band, and a lease
  panel that could not be drawn at all. The pane's 只读 chip, the band
  (只读：当前无人控制, with the way in beside it) and the panel (当前无人控制这个终端,
  取得控制权后你就能继续输入。) are drawn from one answer now, on shells that have
  nobody in them as well as shells that have somebody. Rows read 无人控制 where
  they used to read 控制中 for a shell no one is driving, their holder comes from
  the tab's own lease frames rather than from a broadcast that can be a push
  behind, and the tooltip says 控制端 本窗口 for a shell this window drives instead
  of denying it. A lease the window has not heard about yet is marked with
  nothing at all: the word arrives in a role frame, and a window that read that
  silence as 只读 would flash the mark over every shell a person opens.

## 0.4.11

**Direction C, and a light with one meaning**

`designs/ui-redesign/` held three directions and C — Editor Native — is the one
that was picked, so both clients are drawn from it now rather than from the
system they happened to share: one violet accent for what is chosen or in
control, paper-flat surfaces at 6/10/14px, pills instead of underlines, and the
same near-black terminal sheet in both appearances. The one piece of chrome that
is not words — the light along a shell somebody else is typing into — now means
one thing only.

- **Both clients take the C direction.** The accent moves from teal to violet:
  0.4.9 had moved the desktop off its violet onto the browser's teal so the two
  would match, and the direction that was then chosen is violet, so both are
  violet now. Tabs are pills on a raised strip instead of underlines, the
  composer is one bordered field that lights around the caret rather than a card
  with three controls in it, the key bar is one row of equal keys grouped by
  hairlines, and the sidebar, the session inspector and the settings window are
  redrawn on the same tokens. Colour means one thing each — accent for chosen,
  amber for read-only, red for gone, grey for exited — and where the desktop
  drew decoration that carried nothing, it draws nothing.
- **The terminal's sixteen colours are the app's.** The browser client's theme
  and the desktop's carry the same numbers now: the cursor and the ANSI magenta
  are the accent's violet, the selection is that violet at 28%, and the sheet is
  the same `#0d0d12` in both appearances. A shell should look like it belongs to
  the window around it, and ANSI's sixteen are drawn for a dark ground — which is
  why the light appearance keeps a dark terminal rather than inverting one.
- **The light along a taken shell is drawn under one condition, and it reads as
  light.** It used to come on for any shell another client held, including one on
  a machine the person is not sitting at — a light for a process that is
  nobody-here's, which is the ordinary case rather than news. It is drawn only
  for a shell *this* machine runs with somebody else on its keyboard, which is
  the one thing the words on screen do not say from across the room; a shell on
  another machine keeps its 只读 chip, because that is a fact about the pane you
  are looking at. The effect is one family of colours instead of a spectrum (the
  old cut ran violet, pink, coral and peach around the edge — a second and louder
  vocabulary for a fact the pane was already stating in words) and it is a band
  with no line anywhere in it, falling off on both sides of the pane's edge, the
  way the platform's own agent-takeover glow is drawn, rather than a coloured
  border painted on the pane. Where the desktop asks for less motion the colours
  stop travelling and the light stays.
- **The takeover offer says what pressing it does to the other side.** The panel
  under a shell that is not yours names the holder, says that your typing is not
  reaching the shell, and now says what happens to *them*: they go read-only and
  keep watching the terminal afterwards. A button that takes a keyboard away
  from somebody without saying so is one people learn not to press.

**Fixed**

- **释放控制权 was offered by a window that held nothing.** The browser client
  sets `hidden` on the button, and `hidden` is a bare `display: none` from the
  browser's own stylesheet, which loses to any class that sets a display — and
  every control in that client does. The button sat in the header of a window
  with no session at all. The per-class workarounds the stylesheet had
  accumulated (`.tf-hero[hidden]`, `.tf-inspect[hidden]`, …) are one `[hidden]`
  rule now, so the next control that hides itself stays hidden.
- **The browser client's status line went stale** after the session list changed,
  counting sessions the sidebar no longer showed, and the session inspector could
  not be closed — the button that dismissed it reopened it on the next render.
  Both follow the person now: the count follows the roster, and whether the
  inspector is open is a choice that is remembered while the page is.

## 0.4.10

**Four separate acts, and the ones that were missing**

A client watching a shell could take it over but never give it back, and the
button that said 关闭标签页 ended the process instead. Both are fixed, and the
four things a client can do to a shell are now four buttons on both clients.

- **释放控制权 exists.** The release frame was in the protocol and no client
  ever sent it. A client holding the lease is offered 释放 in the header and in
  the banner; pressing it gives the lease up, leaves the process running and
  leaves the tab watching it. The hub hands the lease to the machine the shell
  runs on, so the shell is not left mute.
- **The hub was throwing a release away.** `handleRelease` dropped the lease and
  then asked `giveAway` to hand it to the machine's own window — which is the
  client that had just released it, because `drop` had just added it to the
  watchers. A release from the machine's own window was a no-op, and that
  window is the one that releases most, since it holds its own shells by
  default.
- **The 会话 panel's × closed the tab and ended the shell.** It was wired to the
  same act as 结束, so "close this tab" killed a process on a machine somewhere
  — for every client watching it, not only the window that clicked. It closes
  the tab now, and 结束 has its own button on the row of the shell it ends.
- **A tab whose shell exited is no longer removed.** The tab is where the
  shell's last output is read, so ending a session is not a reason to lose it.
  The browser client already behaved this way; the desktop app's reconcile did
  not.
- **The desktop app catches up with the browser client's layout.** It gains the
  tab strip, the command composer with its history bar, and the 280px session
  inspector, and its sidebar and terminal take the same measurements the
  browser client uses. Without a composer the window had no way to send Esc,
  Tab, Ctrl or an arrow key at all, because a terminal is a display surface and
  cannot produce them.
- **The app icon is transparent.** `resources/icon.png` had an opaque dark plate
  composited behind the logo, which is what put a dark square in the macOS Dock
  and the Windows taskbar. Every icon is rendered from the one transparent
  source now, and a test reads the pixels so it cannot come back.
- **The release pipeline builds the frontend.** The `cli` job never ran
  `build:web`, so every CLI artifact published so far shipped without the
  browser client embedded — `tfd` answered the browser with nothing.
- **The macOS firewall prompt is explained before it appears.** The 本机服务
  switch says that the system asks once about incoming connections and that
  clicking 允许 settles it for good. A service that failed to start is stated in
  the status bar rather than only logged, and the background update check starts
  off instead of calling out on every launch.

## 0.4.9

**One design, two clients**

The browser client and the desktop app are drawn from the same palette now. A
shell in a tab and a shell in the native window say the same things in the same
words, with the same colours, at the same sizes: one accent for what is chosen
or in control, amber for read-only, red for gone, grey for exited.

- The desktop app's palette moves off its violet onto the teal the browser
  client uses, and its terminal takes the same sixteen ANSI colours, so a shell
  belongs to the window around it rather than looking like a program that
  wandered in. The header band is 44px and the status line 30px, the heights the
  browser client uses, and the status line states the connection as a chip — a
  dot in a wash — the way the browser does.
- A shell in the sidebar carries its state as a badge — 控制中, 只读, 已退出 —
  with the holder's name beside it, and the row's tooltip carries the numbers
  the row has no room for. The empty window says what the machine it would open
  a shell on is doing, rather than showing an empty rectangle.
- The browser client is redrawn on the same system: a cold near-black ground
  with hairlines instead of borders, a sidebar whose rows are one badge and one
  number, tabs whose underline is their state, and a right-hand session
  inspector holding what the daemon said about the shell on screen. A dark and a
  light appearance are both complete, and the choice is remembered per device.
- The composer gained a strip of the commands this tab has sent — real history,
  newest first, one tap back into the box — and says on the same line what the
  Enter key will do. The relay's connect screen names the app, says why it is
  asking, and reports an empty token next to the field instead of doing nothing.
- The phone layout is a drawer and a bigger key bar rather than a second design,
  with touch targets at 44px and the terminal kept whole.

**Fixed**

- The browser client's empty state kept a stale line about a previous machine
  after the roster changed, and said nothing at all once the machine had
  sessions to show.

## 0.4.8

**Desktop app**

- Fixed a race on the relay client's roster callback. The window hands the fleet
  link the place its roster goes, and the link's read loop reads it as frames
  arrive: a window opening while the relay was already pushing a roster read and
  wrote the same field from two goroutines. It is guarded now, both where it is
  set and where it is called, so 设置 → 远程 → 应用 on a live link cannot race an
  arriving roster frame.

## 0.4.7

**Desktop app**

- A new terminal opens in this machine's home directory, not wherever the app
  happened to be started. Started by the window server — Finder, the Windows
  shell — that place is the root of the filesystem, so every new shell opened in
  `/`: a directory nobody chose and everybody leaves again. `-cwd` still says
  otherwise, a client that asks for a directory keeps what it asked for, and a
  shell started on another machine opens in *that* machine's home, which is the
  only home that means anything there. The same applies to terminals started from
  a phone or a browser.

## 0.4.6

**Desktop app**

- The lease follows the shell on screen. A window that opened a shell on another
  machine and then moved the screen on — to 本机, or to any other shell — used to
  leave that machine showing 正在被控制 with a keyboard that no longer worked.
  Switching away lets the lease go, the machine whose shell it is takes it back
  the way it does when a holder's lease lapses, and coming back to that shell
  drives it again without a click. The connection to that machine stays: it is
  the lease that moves, not the link.
- Picking a machine in the 机器 panel brings one of its shells on screen when this
  window has one open there, so 切到本机 means the screen as well as the panels:
  two panes and a pane that disagree about what is being worked on is one answer
  too many.
- The hub hands a lease nobody holds — released, let go by a detach, or simply
  lapsed — to the machine's own window when it is watching. A shell of a machine's
  is that machine's to type in, so a client that stops driving it cannot leave it
  mute with nothing on screen to say why.

## 0.4.5

**Desktop app**

- Control belongs to the window that reached the machine. Opening a shell another
  machine was running left this window read-only — a spectator of a shell it had
  just asked to work in — with the glow and 拿回控制权 drawn on the *controlling*
  machine while the machine being driven kept the keyboard. A tab on another
  machine now takes the lease as it opens: this window types, and the machine
  whose shell it is goes read-only behind the glow, saying who is typing into it.
  拿回控制权 there is still one click, and the hub always honours it.
- 会话 lists the picked machine's shells and nothing else. A tab on another
  machine used to be a row in this machine's list — it looked like one of this
  machine's shells — and picking it moved the panels to that machine, so
  switching tabs landed somewhere else entirely.
- The panel under the glow offers 接管 on a shell of another machine's rather than
  拿回控制权, which is what taking one is when the lease was never this window's.

## 0.4.4

**Desktop app**

- Fixed the 会话 panel not switching shells. Clicking a shell another machine was
  running, when this window already had a tab for it, did nothing at all: the
  click was swallowed and the pane stayed where it was, so a machine's shells —
  the tabs opened to watch them — could not be walked between, while this
  machine's own tabs could. The panel now builds one list either way and picking
  a row selects its tab, opening one only when the shell has no tab yet.
- The 会话 panel says which shell is on screen, with the same accent bar the
  picked machine carries, and picking a shell on another machine moves the 机器
  and 会话 panels with it rather than leaving them on a machine that is no longer
  on screen. ⌘⇧] and ⌘⇧[ walk this window's tabs, wherever each shell runs.

## 0.4.3

**Brand**

- The project wears the mascot: a periwinkle planet with a cream face, a ring and
  a terminal prompt on its brow. It replaces the mark cut out of the logo sheet,
  and it is used as drawn rather than recoloured per surface — which is why the
  app icon, the README's logo and the browser favicon are now the same picture on
  the shell's ink. The mascot's face is cream: on a white tile, or on the
  transparent ground a README has on GitHub's light theme, it would run into the
  page. `resources/brand/make-assets.py` rebuilds all three from the master, and
  takes a new render as its argument when the art changes again.

**Desktop app**

- The ring around a shell being driven from elsewhere is quieter: a thinner line,
  a slower flow and a dimmer bloom than the chat-glow example's composer ring,
  which is drawn to be looked at. (The artist's own reference for this state is
  still pending — this is the window's own reading of "a signal, not a show".)

- A machine taking its shell back ends the connection, on the machine that was
  driving it: the tabs it had open there close, the link to that machine is
  dropped, the panels return to this machine, and the status bar says who took
  the shell back. Before this, the driving window was simply left read-only, with
  a tab on screen that could not type and no word about why.
- The panel under the glow — who holds a shell, and 保持只读 / 拿回控制权 — has a
  width of its own and shorter words. It hangs from the top of a pane, and a pane
  can be narrower than a sentence: it used to run past the terminal it was about.
- The 机器 panel is a list you switch between rather than a nest of shells. This
  machine comes first and is picked by default; picking another machine makes it
  the subject of the 会话 panel below and of that panel's 新建标签页, which now
  starts a shell *on that machine* over the relay and opens it here. The relay
  itself has no row any more — it is the pipe, not a machine — and its state
  moved to the line the header and status bar already carry.
- Reaching the other machines no longer needs a secret typed anywhere. 设置 → 远程
  lost the 客户端令牌 field: a window asks its relay for a short-lived *control
  token*, proving itself with the device credential it already joins the fleet
  with, and that is what its client link presents. Picking a machine the window
  cannot reach yet asks first, in a dialog, so "this window is about to ask the
  relay for reach" is something a person sees and can decline. The fleet's shared
  token is unchanged and is now only what a browser or a phone is given:
  `tfrelay -control-token-ttl` (default 15m) bounds what a machine gets, revoking
  a machine at the relay takes its credentials away at once, and the machine's own
  允许被控制 switch is unchanged.

## 0.4.2

**Desktop app**

- Fixed the window mixing up shells from different machines: frames were routed
  to a tab by session id alone, and session ids are counted per machine — every
  machine has an `s1`. A window driving another machine at the same time as it
  held shells of its own could hand one shell's role frame, exit or close to the
  other shell's tab, which moved the lease marks, the glow and the tab itself.
  Frames are matched by the session object now, and a test drives two desktops
  against each other to hold that.
- The glow around a shell another client is driving is MyGo's shader now: the
  ring from `examples/chat-glow`, compiled for the renderer in front of the
  window and interpreted on the CPU where there is none. The stroked-ring
  approximation it replaces is gone, so the previews show the window as it is
  rather than an impression of it. (`github.com/egoist/mygo` v0.4.8 → v0.5.5.)
- The 机器 panel no longer lists this machine twice. The roster is keyed by the
  device id a machine was enrolled under and the window knows itself by hostname,
  so a machine on a relay showed up as 本机 and again as another machine with its
  own shells — which offered to open its own shells over the relay.
- 服务 → 允许被控制 is the machine's answer to "who may drive my shells". Being
  enrolled with a relay is the consent, so the switch starts on; off, other
  clients — a browser, a phone, another machine — watch and nothing more: no
  lease, no new shells, and a holder that is not this machine's window loses the
  shell in the frame the switch moves. The machine's own window is never
  refused. `tfd` gets the same switch as `-allow-control=false`, and a claim that
  is refused says `control_off` rather than being dropped: the browser client
  shows why and keeps its offer to take control, so a machine that turns it back
  on needs no reload.
- 设置 → 远程 has the 客户端令牌 field that the 0.4.0 notes already promised: the
  token had nowhere to be typed, so the window's second link — the one that makes
  the 机器 panel list anything but this machine — could never come up, and the
  panel told people to fill in a control that was not on the form at all. The
  field names itself after the browser's 访问令牌 prompt, its own line says
  whether the link is up or still waiting for 应用, and the panel stops promising
  machines that cannot appear.
- A machine the window cannot reach says why. `openTab` used to fall back to the
  local hub and attach a remote session id to it, which opened a tab that showed
  nothing and explained nothing; it now declines the tab, and the panel carries
  the reason under the machines.
- A shell another client takes over says so at the moment it happens: the panel
  that names the holder and offers 拿回控制权 opens by itself, and so does the one
  a person gets by typing into a shell that is not theirs — the hub already
  answers a refused keystroke loudly, and now the window says it where the typing
  went instead of only in the log. The header carries the same offer, and the
  机器 panel counts the shells on this machine that other clients are driving, so
  "why is my keyboard not working?" has an answer that does not depend on
  noticing a glow on the pane's edge.

## 0.4.1

- Fixed three data races the Linux race detector found in the code that reaches
  another machine: a stream's number and accepted flag were written under the
  client's lock and read under the stream's own (accepting a stream raced the
  window sending on it); a link's writer was set at construction and cleared by
  the link's reader goroutine with nothing between them; and New Tab opened a
  tab — window state — from a goroutine, racing the frame that draws it, which
  on a Go map is a fatal error rather than a stale read. The tab is now opened on
  the window's own thread and the create frame sent from there.

## 0.4.0

**Taking over a shell on another machine** (`docs/takeover.md`)

- A session's control is a **lease**: `{holder, epoch, deadline}`. Attaching
  without it is read-only, every input frame carries the epoch it was sent
  under, and the owner drops frames whose epoch is past — so input from a
  controller that was just revoked cannot land after the handoff. A lease nobody
  renews lapses, and a silent controller cannot lock the machine out of its own
  shell.
- The desktop window is an ordinary hub client rather than a privileged reader
  of its own sessions: its local link is an in-process pipe (`hub.Pipe`), and a
  remote one is a relay stream (`internal/relayclient`). The hub marks the
  window's own client as the *owner*, so the machine's window takes its shell
  back from anyone, with no confirmation — and a remote client's claim against a
  live holder is refused unless it steals, which the browser client confirms
  first and names the holder in.
- The window reaches the rest of the fleet: enrolled with a relay and holding
  its client token, the 机器 panel lists every machine on that relay with the
  shells it is running, and a shell on another machine opens in a tab here under
  that machine's arbitration. The token is the fleet's client secret and today
  the whole authority over every machine on the relay; a per-device ACL is what
  is still missing.
- A shell another client holds glows around its pane — colours cycling through
  the palette, the shape of mygo's chat-glow example — with a popover naming the
  holder and offering 拿回控制权, and the holder's name on the tab's row. The
  glow runs only while someone else holds the lease, is drawn along the pane's
  edge rather than over the text, and its fade and cycle stand still where the
  desktop asks for less motion.
- The hub announces a created session to its creator *before* the session list
  puts it in front of everyone else, so a client's pending tab becomes the tab
  watching the shell instead of a second tab for the same shell. A departing
  controller's lease is released and the epoch bumped in the same frame, so the
  clients still watching learn the shell is free.

**UI**

- The shell window has a left rail: the open shells, `切换到白天/暗黑`, and the
  settings window. The appearance is saved (`theme`: `light`, `dark`, or unset
  for the system) and the terminal follows it; the settings window's 外观 pane
  offers all three modes.
- New mark, taken from the logo sheet's third row, second column: the app icon
  keeps the sheet's dark tile, the README logo and the web favicon keep the white
  tile with dark ink.
- The desktop window has a palette of its own — the navy ground, the violet for
  what is chosen and the green for what is live — instead of the ui's default
  greys, and the terminal is themed in the same palette rather than its own.
- The strip's two controls moved to its bottom, under the shells: the mark heads
  the strip, and what changes the window itself ends it, so the empty middle no
  longer reads as a control that was forgotten. The fleet's state stays under
  them.
- The column beside the strip is two panels with a draggable line between them:
  会话 on top, 机器 under it. The tab controls moved into the 会话 panel's title
  row as icon buttons — they act on that panel — and the window's header is left
  to naming the shell on screen. Until the line is dragged, the machines' panel
  takes the room its content needs and the shells take the rest, whatever height
  the window is.
- The 机器 panel lists the fleet this window can reach: this machine with the
  service's state, the relay it is enrolled with, and every other machine on
  that relay, each with the shells it is running once the window has looked.
- The settings window is a form: every pane's labels sit in a column beside
  their controls, right-aligned as macOS forms do, instead of stacked full-width
  above them. 应用 is offered only when there is something to apply, the panes
  scroll instead of clipping in a short window, and the 更新 pane hides the
  check button on a build that cannot update itself rather than offering a
  control that does nothing. The 远程 pane gained a 客户端令牌 field, which is
  what lets the window drive the rest of the fleet.
- A window flashes the colour of the appearance it is about to paint, so a light
  window no longer opens with a dark frame around it.

**Browser client**

- Selecting text in a terminal now has the affordance a text surface has: drag a
  range and a 复制 button appears at it — the copy is a choice, not something
  that happens because the pointer was released. Right-click copies the
  selection too, since xterm paints selection in its own layer and the
  browser's own Copy has nothing to act on. `⌘C` / `Ctrl+Shift+C` work as
  before.
- Taking a shell from a live holder asks first: the 接管 button confirms, naming
  the holder, and the claim carries `steal`. A refused claim is answered with
  `lease_held` and a line saying who holds it and until when.

## 0.3.0

**Desktop app**

- The desktop app updates itself. It asks the release feed once a day in the
  background and, when a version is out, shows its notes in a window with
  install / remind me later / skip this version; the download is verified
  against the key built into the app before it replaces anything. macOS also
  gets 检查更新… in the app menu, and every platform gets a 更新 pane in
  设置 (automatic checks and downloads, when it last checked, and a check
  now button).
- Updates replace the app where it is installed, so they work for the dmg,
  the Windows installer and the Linux tarball — a .deb is updated by the
  package manager, and the pane says so instead of offering switches that
  would do nothing. Development builds behave the same way.
- Releases now carry, next to the installers, a signed archive and an
  `update-<target>.json` manifest for every desktop platform, plus delta
  updates from the versions before, so an update is usually a fraction of
  the full download.

## 0.2.0

**Clients (web, desktop, phone)**

- Typing moved out of the terminal into a composer bar under it: text is
  composed locally and sent as one frame on Enter, so a slow link is paid
  once per command instead of once per character. The terminal itself is now
  a display surface (stdin off) — selection and copy keep working.
- Control keys a text box cannot express (Esc, Tab, sticky Ctrl/Alt, arrows,
  C-c, C-d) are injected by the accessory key bar, which is now shown on
  every device rather than phones only.
- The composer carries per-tab drafts and ArrowUp recall of sent commands,
  and it locks with the reason on screen when the session is read-only, dead,
  or the link is down — a command is never queued across a disconnect to run
  later unattended.
- Keystrokes that lose the link on the way out are buffered and replayed on
  reconnect instead of vanishing, input bursts coalesce into one frame, and
  the relay path's base64 encoding is linear rather than one concatenation
  per byte, so large pastes stop janking.

**Brand**

- The logo and the web favicon now sit on a white rounded tile with dark
  ink, so the mark stays legible on dark backgrounds; the README no longer
  needs two scheme-specific logo files.

## 0.1.0

First release: terminals from many machines, one browser tab each.

**Relay (`tfrelay`)**

- Routes WebSocket tunnels between machines running `tfd` and browser, phone or
  desktop clients, one client tab per real session on the host.
- Built-in web admin panel at `/admin/`: first run claims an administrator
  account, then machines, credentials, browser tokens and the CA certificate are
  all managed from the page — no shelling into the relay host.
- Credentials issued in the panel take effect immediately; revoking a machine
  drops its tunnel without a restart.
- Browser tokens can be minted with a lifetime (1 hour to 30 days, or never) and
  rotated from the page.

**Daemon (`tfd`)**

- Owns real PTY sessions on the machine it runs on, and dials out to a relay so
  the machine needs no inbound port.
- Runs PowerShell by default on Windows when a modern one is installed, falling
  back to Windows PowerShell and only then `cmd.exe`.
- `-relay-ca` pins a private CA, which is what a relay behind a self-signed
  certificate requires.

**Desktop (`tfdesktop`)**

- Native window (mygo UI) with a terminal per session, plus a settings window.
- Terminal font family and size, applied to already-open tabs.
- Relay address, device id and secret, CA certificate (`Fetch from server` or a
  file picker) with a live connection status line.
- Local loopback service toggle, so a browser on the same machine can reach the
  same sessions.

**Project**

- `resources/icon.png` is now a real application icon, so Linux desktop entries,
  macOS bundles and Windows executables are branded.
- Release workflow builds desktop installers and command-line tools for macOS,
  Linux and Windows.
