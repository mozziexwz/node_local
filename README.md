# MSBOOST 本地版（node_local）

在自己的电脑上用 SSH 直接部署和管理 MSBOOST 节点与自备中转。SSH 密码或私钥只在本机内存里使用，不经过网站，也不保存到硬盘。

> **状态：v1.0.0 已于 2026-10-22 发布（M0–M3 完成）。** 规划见 [docs/MSBOOST本地版开发规划.md](docs/MSBOOST本地版开发规划.md)，各阶段说明在 [docs/m0](docs/m0/README.md)、[docs/m1](docs/m1/README.md)、[docs/m2](docs/m2/README.md)、[docs/m3](docs/m3/README.md)。

## 能做什么

- 部署 MSBOOST 节点：与网站「部署 MSBOOST」使用同一个安装脚本，部署出的配置文件直接给 MSBOOST 启动器用。
- 保留配置升级/修复节点、取回配置、查看节点状态。
- 自备中转（可加前置机）与中转服务器管理。
- 朋友中转：帮朋友建中转，或用朋友的中转生成新配置。双方只交换 IP 和端口，都不交出 SSH 或节点密码。
- 卸载清理。

中转码、捐赠权益线路、枫叶、工单等需要网站账户的功能，请继续使用网站。

## 下载

在 MSBOOST 网站的「MSBOOST 本地版」页下载。本仓库的 [Releases](https://github.com/mozziexwz/node_local/releases) 里有同样的三个文件：

- 程序 `MSBOOST-Local-<版本>-win-x64.zip`
- 同版本源代码 `MSBOOST-Local-<版本>-source.zip`
- `SHA256SUMS.txt`

程序解压到 MSBOOST 启动器所在的文件夹即可使用，不需要管理员权限。

## 与网站的关系

执行引擎（`internal/executor`）和节点安装脚本（`installers/node/msboost.sh`）原样复制自网站仓库 [mozziexwz/node](https://github.com/mozziexwz/node) 的固定版本，来源和每个文件的 sha256 记录在 [UPSTREAM.json](UPSTREAM.json)。这些文件不要手改，用同步工具更新：

```powershell
go run ./scripts/syncnode -tag v2.7.0 -source ..\MSBOOST.DE
```

本地版自己的补充放在 `internal/executor/local_*.go`，同步时不受影响。CI 会从网站仓库重新生成同步文件并逐字节比对。

## 目录

```text
main.go, app.go, dto.go  Wails 界面入口与前端绑定（about.go：版本与开源许可）
frontend/                界面（React + TypeScript + Vite）
internal/executor/       执行引擎（同步自网站）与 local_*.go 本地补充（进度、升级、状态、朋友中转）
internal/tasks/          任务运行：每台服务器同时一个任务、进度事件、任务记录、朋友中转的分享文字
internal/servers/        服务器列表（local\servers.json，不含密码）
internal/relays/         中转的来源与备注（local\relays.json，不含密码）
internal/store/          数据布局、设置、配置文件命名与保存、启动器识别
internal/logx/           日志（密码打码）
installers/              内嵌的节点安装脚本与 GOST 固定版本（同步自网站）
cmd/msboost-local-cli/   测试用命令行：指纹、部署、取回配置、卸载清理（不随正式版发布）
scripts/syncnode/        从网站仓库同步引擎和脚本
scripts/notices/         生成发布包里的 MSBOOST-Local-THIRD_PARTY_NOTICES.txt
build/package/           发布包里的 MSBOOST-Local-README.txt 模板
docs/                    规划、界面样稿、各阶段说明与发布说明（releases/）
```

## 开发环境

- Windows 10/11 x64，Go 1.27.1，Node.js 24
- 引擎的部分测试要运行 bash 和 python3，只在 Linux 上执行（CI）；在 Windows 上会跳过。

```powershell
go vet ./...
go test ./...
```

构建界面：安装 Wails CLI v2.16.0（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`），在仓库根目录执行 `wails build`，产物为 `build/bin/MSBOOST-Local.exe`。程序是便携的：`configs\` 和 `local\` 都在 exe 旁边，不需要管理员权限。

发布包由 CI 生成：每次构建都产出 `MSBOOST-Local-<版本>-win-x64`；推送 `v*` 标签时再生成草稿 Release，附带程序 zip、同版本源代码和 `SHA256SUMS.txt`，确认后手动发布（见 [docs/m3](docs/m3/README.md)）。

## 许可

本项目使用 [GPL-3.0](LICENSE) 许可。
