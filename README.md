# gq

Windows 局域网文件互传工具。**单个 exe，双击即用，无需安装、无需账号、不联网。**

## 使用

1. 把 `lantransfer.exe` 放到任意目录（U 盘也行）。
2. 两台电脑各双击运行一次。
3. 首次运行 Windows 会弹防火墙提示，**必须点"允许"**，否则对方发现不了你。
4. 看到对方设备后，点击卡片 → 选文件 → 对方点"接收"。
5. 也可以**直接把文件拖进窗口**：拖到某个设备卡片上就发给它；只有一个设备时拖到空白处即可。多台设备时拖到空白处会拒绝发送并提示，不会替你猜。

接收确认弹窗里会显示文件将保存到哪个目录，**点"更改"可以在接收前换位置**（含每个文件的清单）。

接收的文件直接落在 `%USERPROFILE%\Downloads\gq\`，**不再按发送者建子文件夹**；文件名撞了自动加 ` (1)`。顶栏可以改这个目录。

## 便携性

- **分发只有一个文件**：`lantransfer.exe`（7.8 MB），复制到任意 Win10/11 机器双击即用。
- 导入表只有 `kernel32.dll`：无第三方 DLL、无 VC++ 运行库、无安装包。
- 首次运行会在 exe 同目录生成 `lantransfer.json`（约 120 字节）和 `lantransfer.log`；目录不可写时自动改用 `%APPDATA%\gq\`。
- **把整个文件夹拷到另一台电脑会自动更换设备 ID**（配置里记录主机名）。否则两台机器共用同一个 ID，发现时会把对方当成"自己"而互相看不见。
- 唯一外部依赖是系统的 **WebView2 Runtime**（Win10/11 自带）。缺失时退回系统浏览器打开界面，功能不受影响；此时界面顶部会说明原因，并给出安装入口。详见下文「如果打开的是浏览器窗口」。

## 分发：哪些机器需要额外装东西

先看这张表，多数机器不需要做任何事：

| 目标机器 | 需要做的 |
|---|---|
| **Windows 11**（所有版本） | 只拷 exe，双击即用 |
| **Windows 10 1803+**（装过 2022-11 之后的更新） | 只拷 exe，双击即用 |
| Windows Server 2016 / 2019 / 2022 | 先装一次 WebView2 运行时 |
| Win10 / Win11 **LTSC、IoT LTSC** | 同上（微软明确不预装、不推送） |
| 长期离线的机器 | 跑一次看有没有蓝色横幅；有就装 |
| 任何装不上的机器 | 用浏览器模式，功能不受影响 |

判断方法就一条：**程序启动后顶部有没有蓝色提示条**。没有就是原生窗口，不用管。

微软的规则是：Windows 11 全系预装；Windows 10 通过系统更新推送到 1803 及以上；**Server 和 LTSC 是例外，永远不预装**。所以办公室里那批普通电脑基本都是一次性拷 exe 就完事，只有服务器和精简系统需要单独处理。

推荐的 U 盘内容：

```
lantransfer.exe                              7.8 MB   所有机器
MicrosoftEdgeWebView2RuntimeInstallerX64.exe 204 MB   只在缺的机器上装一次
```

装一次即长期有效（之后由 Edge 自动更新），不是每次运行都要装。

## 如果打开的是浏览器窗口

说明这台机器没有可用的 WebView2 Runtime。**功能完全不受影响**：拖拽发送、接收位置选择、进度、多人排队全部正常——界面本来就是网页，浏览器只是换了个外壳。

两点行为差异，都已在代码里处理：

- 浏览器窗口可以自由缩放，布局在 960 / 760 / 480 px 三档自动改为上下堆叠、顶栏换行、收起 logo。
- **关闭这个窗口时后台程序会一起退出**，不会留下占着端口的僵尸进程。否则下次启动会静默占用 45801，变成两个实例同时接收。

界面顶部会显示具体原因（带「安装运行时」按钮），同一个原因也会写进 `lantransfer.log`：

```
webview2: runtime="155.0.4283.62" datapath="C:\Users\...\Local\gq\webview2"
```

- `runtime=""` → 没装。装 [Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) 后即为原生窗口。
- `runtime="1xx.x.x.x"` 但走了浏览器 → 装了但初始化失败，日志下一行有 `Result: 80xxxxxx`，按码对照：

| HRESULT | 含义 | 处理 |
|---|---|---|
| `0x80070490` | 运行时未安装 | 安装运行时 |
| `0x80070002` | 注册表有版本记录，但 `msedgewebview2.exe` 不存在 | **重装运行时**（见下） |
| `0x80070005` | 数据目录不可写 | 检查 `%LOCALAPPDATA%` 权限 |
| `0x80040154` | 组件未注册 | 重装运行时 |
| `0x8007139F` | 运行时版本不匹配 | 重装运行时 |

`0x80070002` 值得单独说：它**不是"没装"**，而是"注册表说有、硬盘上没有"。典型来源是克隆的虚拟机/系统镜像（注册表跟着镜像过来，文件没带）、手工删过 `EdgeWebView` 目录、或 Edge 自动更新后旧版本目录被清掉而注册表没同步。程序现在会直接显示具体缺哪个路径。修复只需重装一次运行时，它会覆盖注册表并补齐文件。

常见于 **Windows Server**（不预装 WebView2，机器名形如 `WIN-XXXXXXXXXXX`）和精简版系统。Edge 浏览器存在不代表 WebView2 Runtime 存在，两者是分开安装的。

### 离线电脑怎么装

微软提供两个安装包，**名字很像，能不能离线用完全不同**：

| 文件 | 大小 | 能否离线 |
|---|---|---|
| `MicrosoftEdgeWebview2Setup.exe` | 1.8 MB | ❌ Bootstrapper，安装时还要联网下载 |
| `MicrosoftEdgeWebView2RuntimeInstallerX64.exe` | 204 MB | ✅ 完整离线包 |

离线机器必须用第二个：

```
https://go.microsoft.com/fwlink/?linkid=2124701
```

1. 在有网的机器上下载（文件名 `MicrosoftEdgeWebView2RuntimeInstallerX64.exe`，签名应为 Microsoft Corporation）。
2. 拷到目标机器，**管理员权限**运行，全程不需要网络。
3. 需要静默安装时：`MicrosoftEdgeWebView2RuntimeInstallerX64.exe /silent /install`
4. 装完重开 gq 即为原生窗口；配置、设备 ID、已接收的文件都不变。

如果之前装过损坏的版本导致安装失败，先到「设置 → 应用 → 已安装的应用」卸载 `Microsoft Edge WebView2 Runtime`，再装一次。

### 为什么不换个不依赖 WebView2 的界面库

WebView2 只是窗口外壳，界面本身是 HTML。要彻底摆脱它，得把界面换成 Win32 控件或自绘：

| 方案 | 依赖 | 界面工作 | 结果 |
|---|---|---|---|
| WebView2（当前） | 系统运行时 | 无 | 现在的样子 |
| `lxn/walk` | 无 | **全部重写** | 系统原生控件，观感更朴素，库维护不活跃 |
| `fyne` | OpenGL + CGO | **全部重写** | 自绘风格，中文字体需内嵌，体积 +10 MB 起 |

现有界面里的拖拽落点、进度动画、排队弹窗、空状态都要在另一套控件模型里重做一遍，而功能不会多出任何一项——差的只是窗口图标和任务栏归属。所以除非 WebView2 在那台机器上完全装不上，不建议走这条路。

WebView2 的浏览器数据目录固定使用 `%LOCALAPPDATA%\gq\webview2`，不会在 exe 旁边生成，也不受 exe 所在位置是否可写影响。

## 功能

| 功能 | 说明 |
|---|---|
| 自动发现 | UDP 组播 `239.255.72.89:45800`，1.5 秒一次心跳 |
| 兜底发现 | 每 60 秒扫一遍本机所在 /24 网段（应对 WiFi AP 隔离 / 禁组播） |
| 传输 | HTTP 直连，单文件流式，多文件串行 |
| 拖拽发送 | 文件拖进窗口即可发送，落点是设备卡片；实时高亮目标 |
| 接收位置 | 确认弹窗内直接看到并可更改，切换后立即生效 |
| 接收确认 | 弹窗显示文件清单，可拒绝；60 秒不响应自动取消 |
| 中文文件名 | 支持，非法字符自动替换 |
| 重名处理 | 自动加 ` (1)` 后缀，不覆盖已有文件；文件直接落接收目录，不建子文件夹 |
| 断点续传 | **暂不支持**（按计划留到下一版） |

## 多人同时使用

| 场景 | 支持情况 |
|---|---|
| N 台设备互相可见 | ✅ 设备数无上限，组播 + 扫描 |
| 一台同时**收**多人 | ✅ 确认请求排队（最多 8 个），逐个接受后可并发接收 |
| 一台同时**发**多人 | ✅ 每次点击独立任务，可并行发出 |
| 拖拽**指定发给谁** | ✅ 拖到哪张卡片就发给哪台，只发那一台 |
| 多人同名设备 | ✅ 自动加 IP 区分；IP 也相同时再加端口 |
| 同一秒、同一发送者多次 | ✅ 接收目录自动加 ` (1)` 后缀 |

三条真实物理约束（不是程序限制）：

- **带宽是共享的**：千兆网总吞吐约 110 MB/s，同时传 3 个文件是各 ~35 MB/s，不会叠加。
- **磁盘是共享的**：并发写同一块盘会互相拖慢，机械硬盘尤其明显。
- **接收端串行写**：同一个 session 内多文件串行（先把一个传完再传下一个），避免小文件抢占带宽。

## 对方发不过来 / 只有单向可见

这是 Windows 防火墙拦了**入站**，不是程序问题。典型表现：你能把文件推给对方，对方却看不到你、也发不过来。

### 最省事的修法：点顶栏「放行防火墙」

界面右上角有 **放行防火墙** 按钮。点一下 → Windows 弹 UAC → 点「是」，程序会以管理员身份给自己加一条入站规则（TCP+UDP，**只允许同网段**访问本程序），加完按钮变成绿色的「防火墙已放行」。

**如果按钮是红色的「修复防火墙拦截」**，说明这台机器上有一条针对本程序的**阻止**规则（通常是以前在防火墙弹窗上点了「取消」留下的）。Windows 里阻止规则优先级高于放行规则，所以光加放行没用——按钮会先把该程序的全部入站规则删掉再重新加，删干净了才变绿。这一点是实测踩出来的：第一版只加放行规则，按钮变绿了但对方依然连不进来。

- 每台机器**只需要点一次**，规则长期有效，不用管理员命令行
- 只放开这一个程序的入站，不是关防火墙；`remoteip=localsubnet` 限定同网段
- 不需要重启程序，加完对方 3 秒内就能连进来

按钮的状态来自实际查询（`netsh advfirewall firewall show rule name="gq LAN"`），已经放行过的机器启动时就直接显示绿色。

### 想手工确认或命令行处理

先确认（两条都不需要管理员）：

```powershell
# 1) 本机 192.168.x.x 所在网卡，是不是被识别成 Public
Get-NetConnectionProfile | Select-Object Name, InterfaceAlias, NetworkCategory

# 2) 有没有本程序的入站放行规则
Get-NetFirewallRule -DisplayName "gq LAN*" | Select DisplayName, Enabled, Direction, Action, Profile
```

如果第 1 条显示 **Public**、第 2 条**什么都没有**，就是它了：公用配置文件默认拒绝一切入站。离线局域网常见「未识别的网络」，Windows 会直接按 Public 处理。

修复（**管理员** PowerShell，路径改成 exe 实际位置）：

```powershell
$exe = "D:\gq\bin\lantransfer.exe"
# 先删掉可能存在的阻止规则：Block 优先级高于 Allow，留着会一直压着
Get-NetFirewallRule | Where-Object { $_.DisplayName -like "*lantransfer*" } | Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName "gq LAN TCP" -Direction Inbound -Program $exe -Protocol TCP -Action Allow -Profile Any -RemoteAddress LocalSubnet
New-NetFirewallRule -DisplayName "gq LAN UDP" -Direction Inbound -Program $exe -Protocol UDP -Action Allow -Profile Any -RemoteAddress LocalSubnet
```

**为什么必须三条一起**：Windows 防火墙里显式 `Block` 规则**优先于** `Allow` 规则。有的机器点过防火墙弹窗的「取消」，就会留下一条针对本程序的 Public/Block 规则；此时只加 Allow 规则毫无效果，必须先删。

装好后不需要重启程序，对方 3 秒内（组播）或 60 秒内（扫描兜底）就能看到你。

## 编译

需要 Go 1.22+（无需 gcc / Node / Visual Studio）：

```bat
build.bat
```

产物 `bin\lantransfer.exe`，约 7.8 MB，`CGO_ENABLED=0` 静态链接，不依赖任何运行时。

## 结构

```
main.go                     入口、日志
window_windows.go           WebView2 窗口（失败则退回系统浏览器）
internal/config/            便携配置（exe 同目录，不可写则 %APPDATA%）
internal/app/
  app.go                    状态、生命周期、端口选择
  discovery.go              组播发现 + /24 扫描
  mcast_windows.go          Windows 组播 socket（必须 bind INADDR_ANY）
  http.go                   API 路由与收发处理
  send.go                   发送端
  drop.go                   拖拽发送（浏览器文件流转发，不落盘）
  firewall_windows.go       「放行防火墙」：netsh 加规则 + UAC 提权
  pick_windows.go           Win32 文件/文件夹对话框
internal/ui/assets/         内嵌 HTML/CSS/JS
docs/                       架构设计与调研
test/                       开发期测试脚本（见 test/README.md）
```

## API

所有接口同时服务本机 UI 和局域网对端，端口 45800（被占用则顺延到 45811）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/state` | 完整状态快照（设备、传输、待确认队列 `offers[]`） |
| GET | `/api/info` | 设备身份（扫描探测用） |
| POST | `/api/offer` | 发送方提交文件清单，阻塞等待用户确认 |
| POST | `/api/upload?session=&idx=` | 裸二进制上传单个文件 |
| POST | `/api/done?session=` | 结束会话 |
| POST | `/api/accept` / `/api/reject` | UI 确认/拒绝 |
| POST | `/api/pick-send?peer=&mode=` | 弹原生对话框并发送 |
| POST | `/api/send-path` | 指定绝对路径发送（**仅 localhost**） |
| POST | `/api/drop-prepare` | 拖拽：协商清单，返回 job（**仅 localhost**） |
| POST | `/api/drop-file?job=&idx=` | 拖拽：请求体即文件字节，流式转发（**仅 localhost**） |
| POST | `/api/drop-done?job=` | 拖拽：结束会话（**仅 localhost**） |
| POST | `/api/recvdir` | 弹目录选择框，改接收目录（写入配置） |
| GET | `/api/firewall-status` | 查入站规则是否存在（只读，无需管理员） |
| POST | `/api/firewall` | 提权添加入站规则（**仅 localhost**，会弹 UAC） |

## 环境变量（调试用）

| 变量 | 作用 |
|---|---|
| `LT_CONFIG` | 指定配置文件路径，用于同机多实例测试 |
| `LT_PORT` | 指定起始监听端口 |
| `LT_AUTO_ACCEPT=1` | 跳过接收确认，自动接受（无人值守场景） |
| `LT_FORCE_BROWSER=1` | 强制走浏览器窗口，用于验证降级提示 |

## 已知限制

- 只支持 Windows（代码结构可移植，但对话框和组播是 Win32 实现）。
- 不支持断点续传：中断需重传。
- 目录发送会**扁平化**（保留文件名，不保留层级）。
- **拖拽不支持文件夹**（浏览器拖入只给文件对象），需要发文件夹时用卡片上的"文件夹"按钮。
- 不校验文件哈希，只按字节数校验。
- 局域网内任何设备都能访问 UI 与 API，无鉴权（个人可信网络使用）。
- 多网卡（VPN / VMware / WSL）已从扫描中排除，但组播只在物理网卡上 join。

## 许可

MIT，见 [LICENSE](LICENSE)。

