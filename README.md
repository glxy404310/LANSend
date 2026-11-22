# LANSend

**桌面稳定版 [v1.7.0](https://github.com/glxy404310/LANSend/releases/tag/v1.7.0)** · [下载 Windows x64](https://github.com/glxy404310/LANSend/releases/download/v1.7.0/LANSend-Windows-x64.zip) · [下载 macOS 通用版](https://github.com/glxy404310/LANSend/releases/download/v1.7.0/LANSend-macOS-universal.zip) · [SHA-256 校验值](https://github.com/glxy404310/LANSend/releases/download/v1.7.0/SHA256SUMS.txt)

LANSend 是面向 Windows 与 macOS 的点对点文件互传工具（支持 Windows↔Windows、macOS↔macOS 及 Windows↔macOS 1 对 1 互传），使用 HTTPS 双向证书校验传输文件。支持通过资源管理器或 Finder 右键菜单快速发送，也支持内置 Tailscale（`tsnet`）跨网络直连。文件保存后，接收端默认弹出前台小窗口以便打开文件或收件夹，也可切换为静默接收。

## 支持平台

| 平台 | 架构 | 说明 |
| --- | --- | --- |
| Windows 10 / 11 | x64 (`amd64`)、ARM64 (`arm64`) | Release 提供 Windows x64 安装包；源码同时支持构建 ARM64 |
| macOS 11.0+ | Apple Silicon (`arm64`) + Intel (`x86_64`) | Release 提供通用二进制应用包 (`LANSend.app`) |

## 安装与运行

### 下载安装

1. 从上方或 GitHub Release 下载对应平台的压缩包与 `SHA256SUMS.txt`（可选校验）。
2. **Windows**：解压后双击 `LANSend.exe`，按提示完成当前用户安装（无需管理员权限），之后从开始菜单打开。
3. **macOS**：解压后将 `LANSend.app` 拖入「应用程序」并双击打开，首次启动会自动配置本机接收服务与 Finder 快速操作。

> macOS 应用使用本地 ad-hoc 签名（未公证），Windows 安装包未做开发者签名。安装、升级、校验与卸载细节见 [使用指南](docs/使用指南.md)。

### 日常使用

- **LANSend 码双向配对**（双方均需支持此功能）：在「配对与设置」点击「复制 LANSend 码」，将整串码发给同一局域网内的另一台电脑，在对方电脑复制后点击「从剪贴板配对」。配对码 5 分钟内有效，重新生成会替换旧码。
- **不同网络**：在「我的电脑 → 远程连接」登录同一个 Tailscale 账号，列表会结合对方主机名（或 Tailscale 自定义设备名）与平台短标识展示同账号在线设备，选中后点击「配对所选电脑」即可双向配对。
- **启用设备**：完成局域网或远程配对后，在两台电脑上分别打开「配对与设置 → 已配对设备 → 管理…」，勾选对方并保存。从旧版升级后，也请在这里检查原有设备是否已启用。
- **发送文件或文件夹**：在设备列表中设好发送目标后，即可从主窗口选择或拖入文件、文件夹。临时发给另一台电脑时，在 Windows 的「发送到」或 macOS Finder 的「快速操作 / 服务」中选择对应的「LANSend · 设备名称」。发送文件夹会保留目录结构和空目录，失败项可在传输队列中单独重试。
- **接收方式**：文件默认保存至 `Downloads/LANSend`（重名自动编号），也可在「配对与设置」的「收件目录」中自定义保存路径或随时恢复默认（修改后即时生效）。同一页面支持切换前台弹窗或静默接收，重启后沿用；关闭主窗口后接收服务仍在当前登录会话中运行。

## 更新日志（Release Notes）

### v1.7.0

- 支持同时配对多台电脑。在「配对与设置 → 已配对设备 → 管理…」中，可以逐台启用或停用、编辑备注、设置当前发送目标，以及取消配对。每次传输仍发给一台设备。
- Windows 资源管理器和 macOS Finder 的右键菜单会列出已启用的设备。选择「LANSend · 设备名称」即可直接发送，无需先切换主窗口的发送目标。设置备注后，菜单会使用备注名。
- 首页右上角显示「已连接 N 台设备」，统计已启用且当前可达的设备。局域网和 Tailscale 远程连接均纳入统计，同一台设备只计一次。
- 简化「配对与设置」页面。本机名称按需编辑，设备备注统一在设备列表中修改。收件目录可直接选择复制，长路径会保留末尾文件夹名。
- 修复 Windows 深色模式下，启动和首次切换页面时可能短暂出现白屏、黑块或控件底色不一致的问题。

### v1.6.5

- 支持修改本机名称、为配对设备设置备注，并在首页查看对方的在线状态。
- 同一局域网内设备 IP 变化后，可自动找回设备并核对原配对证书，无需重新配对。
- 新增传输队列，可查看每项进度和结果，单独重试失败项。双方更新后，可直接发送整个文件夹，保留子目录和空目录。

### v1.6.4

- 支持复制、粘贴 LANSend 码，在局域网内完成双向配对。

### v1.6.3

- 更新了部分界面和内置文字。

### v1.6.2

- Windows 与 macOS「配对与设置」页新增自定义收件目录设置：支持通过系统文件夹选择器更改保存位置并校验写入权限，也可随时一键恢复默认目录（`Downloads/LANSend`），修改后对后续来件即时生效，无需重启后台接收服务。
- 同步更新主窗口「打开收件箱」与前台收件弹窗的「打开文件 / 打开收件夹」行为，确保切换目录前后收到的文件均可准确打开。

### v1.6.1

- 自动检测本机主物理网卡，「配对与设置」页仅显示单个真实的局域网 IPv4 地址，自动过滤 WSL / Hyper-V、VMware / VirtualBox、Clash TUN、Tailscale 及 `169.254.x.x` 等虚拟或无效地址，并在切换页签与网络变化时刷新。
- 支持 Windows↔Windows、macOS↔macOS 与 Windows↔macOS 1 对 1 互传：统一窗口标题与右键菜单入口为「发送到已配对设备」/「用 LANSend 发送」，升级时自动清理旧入口。
- 优化「我的电脑」设备列表展示，结合对方主机名（或 Tailscale 自定义设备名）与平台短标识区分同账号在线设备。

### v1.6.0

- Windows 与 macOS 新增前台收件弹窗及「静默接收」设置，连续收件合并展示，支持直接打开文件或收件夹。
- macOS 主应用直接托管包内接收引擎，移除旧版独立通知助手。

### v1.5.1

- 支持 Windows 与 macOS 点对点 HTTPS 双向证书校验传文件，集成资源管理器 / Finder 右键快速发送。
- 支持局域网手动配对，以及基于内置 Tailscale（`tsnet`）的跨网络同账号设备发现与一键双向配对。
- 支持后台常驻接收，默认保存至 `Downloads/LANSend`。

## 从源码构建

需要 **Go 1.27.4+**；构建 macOS 应用还需在 macOS 上安装 **Xcode Command Line Tools**。在仓库根目录执行：

| 目标 | 命令 | 输出 |
| --- | --- | --- |
| Windows x64 / ARM64（在 macOS 或 Linux 交叉编译） | `make windows` | `build/windows-amd64/LANSend.exe`、`build/windows-arm64/LANSend.exe` |
| macOS 通用应用（需在 macOS 运行） | `make macos` | `build/LANSend.app` |
| 清理构建目录 | `make clean` | 移除 `build/` 与 `dist/` |

在 Windows 本机使用 PowerShell 构建当前架构程序：

```powershell
New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -ldflags="-s -w -H=windowsgui" -o build/LANSend.exe .
```

> macOS 构建脚本会对应用执行本地签名；如仓库位于云同步目录导致签名失败，可设置 `LANSEND_BUILD_DIR` 指向同步目录之外的路径。
