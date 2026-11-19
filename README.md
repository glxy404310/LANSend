# LANSend

LANSend 是 Windows 和 macOS 之间的文件互传工具。选中文件后，可以从资源管理器或 Finder 的右键菜单发送。文件保存成功后，点击桌面通知就能在接收端定位到它。

## 使用

- **同一局域网**：在「设备配对」中输入对方的地址和校验码。有线与 Wi-Fi 设备可以互传。
- **不同网络**：在「我的电脑 → 远程连接」登录同一个 Tailscale 账号。LANSend 内置 tsnet，会列出同账号的在线电脑。在一端点「配对所选电脑」，两端就能互传。
- **收件**：默认保存在用户主目录的 `Downloads/LANSend`。关闭主窗口后，接收器仍可在登录期间收件。

安装、升级、通知和卸载步骤见[使用指南](docs/使用指南.md)。

## 从源码构建

使用 Go 1.27.4 或更新版本。在仓库根目录运行命令，Go 会按 `go.mod` 和 `go.sum` 获取依赖。

| 目标 | 构建命令 | 输出 |
| --- | --- | --- |
| Windows x64 和 ARM64 | 在 macOS 或 Linux 上运行 `make windows` | `build/windows-amd64/LANSend.exe` 和 `build/windows-arm64/LANSend.exe` |
| macOS 通用应用 | 在装有 Xcode Command Line Tools 的 Mac 上运行 `make macos` | `build/LANSend.app` |

在 Windows 本机用 PowerShell 构建当前架构的图形程序：

```powershell
New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -ldflags="-s -w -H=windowsgui" -o build/LANSend.exe .
```

macOS 构建脚本会进行本地签名。如果在云同步目录中遇到签名错误，可将 `LANSEND_BUILD_DIR` 设为同步目录之外的构建路径。

## 测试

```sh
go test ./...
go test -tags integration -run '^TestEmbeddedTransfer$' -timeout 120s .
```

第一条覆盖传输、证书校验、配对、路由选择和错误提示。第二条启动本地测试控制器、中转服务及两个临时节点，验证自动发现、双向配对和文件内容。测试需要允许监听本机端口。

## 代码入口

| 路径 | 内容 |
| --- | --- |
| `main.go`、`receiver.go` | 命令入口、配对、HTTPS 发送和接收 |
| `embedded.go`、`account_pair.go`、`remote.go` | 内置 Tailscale、同账号设备发现、远程路由 |
| `desktop_windows.go`、`windows-ui/` | Windows 原生界面、安装集成、收件通知和图标 |
| `mac-desktop/`、`mac-helper/` | Mac 主窗口、Finder 快速操作、通知助手及构建脚本 |
| `*_test.go` | 单元测试和带 `integration` 标签的双节点测试 |

Windows 的两个 `.syso` 文件是 Go 链接器使用的图标与程序清单资源，随源码提交。修改 `windows-ui/LANSend.ico` 或 `windows-ui/LANSend.manifest` 后，可在仓库根目录重新生成：

```sh
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest windows-ui/LANSend.manifest -ico windows-ui/LANSend.ico -o rsrc_windows_amd64.syso
go run github.com/akavel/rsrc@v0.10.2 -arch arm64 -manifest windows-ui/LANSend.manifest -ico windows-ui/LANSend.ico -o rsrc_windows_arm64.syso
```

## 版本管理

当前基线为 `v1.5.1`。源码、平台资源和测试随 Git 管理。各平台安装包适合附在对应标签的 GitHub Release 中。分发二进制时应随包保留所含第三方依赖的许可文件。
