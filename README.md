# LANSend

**当前版本 `v1.6.1`** · [Windows x64 ZIP](https://github.com/glxy404310/LANSend/releases/download/v1.6.1/LANSend-Windows-x64.zip) · [macOS 通用 ZIP](https://github.com/glxy404310/LANSend/releases/download/v1.6.1/LANSend-macOS-universal.zip) · [SHA-256 校验值](https://github.com/glxy404310/LANSend/releases/download/v1.6.1/SHA256SUMS.txt)

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

- **同一局域网**：「配对与设置」页会自动检测并显示本机主物理网卡的局域网 IPv4，双方输入对方地址与 12 位校验码保存配对即可（默认端口 `TCP 53318`）。
- **不同网络**：在「我的电脑 → 远程连接」登录同一个 Tailscale 账号，列表会结合对方主机名（或 Tailscale 自定义设备名）与平台短标识展示同账号在线设备，选中后点击「配对所选电脑」即可双向配对。
- **发送文件**：在主窗口选择或拖入文件，或通过 Windows 资源管理器右键「发送到 → 用 LANSend 发送」/ macOS Finder 右键「快速操作 / 服务 → 用 LANSend 发送」发送（从旧版升级时会自动清理原「发送到 Mac」/「发送到 Windows」入口）。文件夹请先压缩。
- **接收方式**：文件默认保存至 `Downloads/LANSend`（重名自动编号），也可在「配对与设置」的「收件目录」中自定义保存路径或随时恢复默认（修改后即时生效）。同一页面支持切换前台弹窗或静默接收，重启后沿用；关闭主窗口后接收服务仍在当前登录会话中运行。

## 更新日志（Release Notes）

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
