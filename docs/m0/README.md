# M0 实机测试：引擎移植

目的：确认搬到本地的引擎能在 Windows 上直接 SSH 到 VPS，完成部署、取回配置和卸载清理，部署出的配置能被启动器使用。这一阶段还没有界面，用测试命令行 `msboost-local-cli.exe` 完成。

## 准备

- 一台临时 VPS：Debian 11/12/13，有公网 IPv4，root 能用密码或密钥登录。最好是刚装好的系统，测试最后会卸载。
- `msboost-local-cli.exe`：从 CI 构件 `msboost-local-cli-windows-x64` 下载，或在仓库根目录执行 `go build -o build/bin/msboost-local-cli.exe ./cmd/msboost-local-cli`。
- 在 **PowerShell 或命令提示符** 里运行。Git Bash 等终端没法隐藏输入的密码，程序会拒绝读取。
- MSBOOST 启动器 v1.1.0。

下文命令里的 `203.0.113.10` 是示例 IP，换成你的 VPS 公网 IP；`SHA256:第1步的指纹` 换成第 1 步显示的整段指纹。不要加尖括号或引号：命令提示符会把 `<`、`>` 当成重定向，报「命令语法不正确」。用密钥登录时，每条命令再加 `-key 私钥文件路径`；私钥有口令时程序会提示输入。

## 步骤

1. **读取指纹**

   ```powershell
   .\msboost-local-cli.exe fingerprint -host 203.0.113.10
   ```

   服务商控制台能看到 SSH 指纹的话，核对一致。

2. **部署**

   ```powershell
   .\msboost-local-cli.exe deploy -host 203.0.113.10 -fingerprint SHA256:第1步的指纹
   ```

   - 输入 root 密码（屏幕不显示），等几分钟。
   - 预期结果为 `succeeded`，`服务：running`，`本地自测：passed`。
   - `公网 TCP` 测的是这台电脑到节点端口；显示 `unreachable` 时检查服务商安全组。
   - `BBR` 显示 `enabled` 或 `unavailable` 都正常（后者表示内核不支持，不影响使用）；显示 `review_required` 时需要人工检查网络参数。
   - 当前目录生成 `直连-203.0.113.10.json`。

3. **启动器验证**：把 `直连-203.0.113.10.json` **复制**（不要剪切，第 4 步还要用）到启动器的 `configs` 文件夹，在「节点配置」里选中。「网络检测」里「当前到游戏服务器 IPv4」应显示 VPS 的 IP，两项延迟正常；有条件再开始加速进一次游戏。

4. **取回配置**

   ```powershell
   .\msboost-local-cli.exe fetch -host 203.0.113.10 -fingerprint SHA256:第1步的指纹 -out 取回.json
   fc.exe /b 直连-203.0.113.10.json 取回.json
   ```

   预期 `fc` 显示两个文件没有差异。

5. **断线后取回（可选）**
   1. 再执行一次第 2 步。进度中断开电脑网络约 1 分钟，或者按 Ctrl+C。
   2. 恢复网络，等 5 分钟后执行第 4 步的 `fetch`。预期拿到新配置：用户名、密码、端口都和第 2 步不同。
   3. 用新配置在启动器里验证能连上。

6. **指纹不符（可选）**：把指纹的最后一个字符改掉再执行 `fetch`。预期提示「SSH 主机指纹已变化，操作已中止」。主机密钥核对在登录之前，密码不会发给这台服务器。

7. **卸载清理**

   ```powershell
   .\msboost-local-cli.exe cleanup -host 203.0.113.10 -fingerprint SHA256:第1步的指纹
   ```

   核对列出的路径后输入 `yes`，预期结果为 `succeeded`。再执行一次 `fetch`，应提示「这台服务器上没有找到 MSBOOST 节点」。

## 回报

把每一步的屏幕输出发回来。不要发 SSH 密码，也不要发配置文件内容：配置里有节点密码。

## 测试记录

### 2026-10-22，dev-c0d9b32，命令提示符，密码登录

| 步骤 | 结果 |
|---|---|
| 1 读取指纹 | 通过（ecdsa-sha2-nistp256） |
| 2 部署 | 通过：服务 running，本地自测 passed，公网 TCP reachable，BBR enabled |
| 3 启动器 v1.1.0 | 网络检测通过：游戏服务器 IPv4 为 VPS 的 IP，到中转约 152 ms，到游戏约 180 ms；部署的配置和取回的配置各测两次。没有进游戏 |
| 4 取回配置 | 通过：与部署时的配置逐字节一致（`cmp` 核对） |
| 5 断线后取回 | 未做 |
| 6 指纹不符 | 通过：`ssh_host_changed` |
| 7 卸载清理 | 通过：列出并删除 8 项（该机没有 UFW/firewalld，所以没有防火墙项）。清理后再取回未做 |
| 其他 | 密码输错一次，正确报 `ssh_auth`；重输后成功 |

文档问题：示例命令里的 `<IP>` 被照抄，命令提示符报「命令语法不正确」，已改成示例 IP；第 3 步原写「放进」，测试时文件被剪切走，第 4 步的 `fc` 无法比对，已改成「复制」。
