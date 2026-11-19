# LANSend

**当前版本 `v1.6.0`** · [Windows x64 ZIP](https://github.com/glxy404310/LANSend/releases/download/v1.6.0/LANSend-Windows-x64.zip) · [macOS 通用 ZIP](https://github.com/glxy404310/LANSend/releases/download/v1.6.0/LANSend-macOS-universal.zip) · [SHA-256 校验值](https://github.com/glxy404310/LANSend/releases/download/v1.6.0/SHA256SUMS.txt)

LANSend 是 Windows 与 macOS 之间的点对点文件互传工具，使用 HTTPS 双向证书校验传输文件。支持通过资源管理器或 Finder 右键菜单快速发送，也支持内置 Tailscale（`tsnet`）跨网络直连。文件保存后，接收端默认弹出前台小窗口以便打开文件或收件夹，也可切换为静默接收。

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

- **同一局域网**：打开「配对与设置」，输入对方的局域网地址与 12 位校验码保存配对（默认端口 `TCP 53318`）。
- **不同网络**：在「我的电脑 → 远程连接」登录同一个 Tailscale 账号，选中同账号在线设备后点击「配对所选电脑」即可双向配对。
- **发送文件**：在主窗口选择或拖入文件，或通过 Windows 资源管理器右键「发送到 → 发送到 Mac」/ macOS Finder 右键「快速操作 / 服务 → 发送到 Windows」发送。文件夹请先压缩。
- **接收方式**：文件默认保存至 `Downloads/LANSend`（重名自动编号）。在「配对与设置」可切换前台弹窗或静默接收，重启后沿用；关闭主窗口后接收服务仍在当前登录会话中运行。

## 从源码构建

需要 **Go 1.27.4+**；构建 macOS 应用还需在 macOS 上安装 **Xcode Command Line Tools**。在仓库根目录执行：

| 目标 | 命令 | 输出 |
| --- | --- | --- |
| Windows x64 / ARM64（在 macOS 或 Linux 交叉编译） | `make windows` | `build/windows-amd64/LANSend.exe`、`build/windows-arm64/LANSend.exe` |
| macOS 通用应用（需在 macOS 运行） | `make macos` | `build/LANSend.app` |
| 清理构建与打包产物 | `make clean` | 移除 `build/` 与 `dist/` |

在 Windows 本机使用 PowerShell 构建当前架构图形程序：

```powershell
New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -ldflags="-s -w -H=windowsgui" -o build/LANSend.exe .
```

- macOS 构建脚本会对应用执行本地签名；如仓库位于云同步目录导致签名失败，可设置 `LANSEND_BUILD_DIR` 指向同步目录之外的路径。
- `rsrc_windows_amd64.syso` 与 `rsrc_windows_arm64.syso` 为 Windows 图标与程序清单资源，随源码提交。修改 `windows-ui/LANSend.ico` 或 `windows-ui/LANSend.manifest` 后可在根目录重新生成：

```sh
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest windows-ui/LANSend.manifest -ico windows-ui/LANSend.ico -o rsrc_windows_amd64.syso
go run github.com/akavel/rsrc@v0.10.2 -arch arm64 -manifest windows-ui/LANSend.manifest -ico windows-ui/LANSend.ico -o rsrc_windows_arm64.syso
```

## Release 产物与发布流程

### 产物清单

在完成 Windows 与 macOS 构建后，运行 `make package`（即 `python3 scripts/package.py build/windows-amd64/LANSend.exe build/LANSend.app dist`），会在 `dist/` 目录生成以下 Release 产物：

| 文件 | 内容 |
| --- | --- |
| `LANSend-Windows-x64.zip` | `LANSend.exe`、`THIRD-PARTY-NOTICES.txt` |
| `LANSend-macOS-universal.zip` | 通用架构 `LANSend.app`、`THIRD-PARTY-NOTICES.txt` |
| `SHA256SUMS.txt` | 上述两个 ZIP 包的 SHA-256 校验清单 |

`scripts/package.py` 会从本机 `GOROOT` 与 `GOMODCACHE` 收集 Go 标准库及所含第三方依赖的许可文件并写入 `THIRD-PARTY-NOTICES.txt`。

### 发布流程

1. 按目标版本更新 `mac-desktop/Info.plist` 中的 `CFBundleShortVersionString` / `CFBundleVersion` 及本文档顶部下载链接。
2. 构建双端产物：`make windows && make macos`。
3. 打包并生成校验值：`make package`。
4. 在 GitHub 创建对应版本标签的 Draft Release，上传 `dist/` 下的三个文件，记录该 Draft Release 的数字 ID。
5. 在 GitHub Actions 手动运行 [Verify release archives](.github/workflows/verify-release.yml)（传入 `release_id`），在 Windows 与 macOS runner 上校验 `SHA256SUMS.txt`、包内文件完整性、`THIRD-PARTY-NOTICES.txt` 以及 macOS 签名与通用架构（`arm64` / `x86_64`）。
6. 校验通过后正式发布该 Release。

## 项目结构

| 路径 | 说明 |
| --- | --- |
| `main.go`、`receiver.go`、`send_error.go` | CLI 入口、证书配对、HTTPS 收发与错误提示 |
| `embedded.go`、`account_pair.go`、`remote.go` | 内置 Tailscale (`tsnet`)、同账号设备发现与路由选择 |
| `desktop_windows.go`、`receipt_windows.go`、`notify_windows.go`、`windows-ui/` | Windows 原生界面、收件弹窗、安装脚本与图标清单 |
| `desktop_other.go`、`notify_darwin.go`、`notify_other.go`、`reveal_other.go`、`peerlock_*.go` | 跨平台适配与文件锁 |
| `mac-desktop/` | macOS 原生应用、Finder 快速操作与构建脚本 |
| `scripts/package.py` | Release 打包、第三方许可汇总与 `SHA256SUMS.txt` 生成 |
| `.github/workflows/verify-release.yml` | Release 候选产物校验工作流 |
| `docs/使用指南.md` | 详细安装、使用、升级、校验与卸载说明 |
