# M0 实机测试：引擎移植

目的：确认搬到本地的引擎能在 Windows 上直接 SSH 到 VPS，完成部署、取回配置和卸载清理，部署出的配置能被启动器使用。这一阶段还没有界面，用测试命令行 `msboost-local-cli.exe` 完成。

## 准备

- 一台临时 VPS：Debian 11/12/13，有公网 IPv4，root 能用密码或密钥登录。最好是刚装好的系统，测试最后会卸载。
- `msboost-local-cli.exe`：从 CI 构件 `msboost-local-cli-windows-x64` 下载，或在仓库根目录执行 `go build -o build/bin/msboost-local-cli.exe ./cmd/msboost-local-cli`。
- 在 **PowerShell 或命令提示符** 里运行。Git Bash 等终端没法隐藏输入的密码，程序会拒绝读取。
- MSBOOST 启动器 v1.1.0。

下文的 `<IP>` 换成 VPS 的公网 IP，`<指纹>` 换成第 1 步得到的 `SHA256:...`。用密钥登录时，每条命令加 `-key <私钥文件>`；私钥有口令时程序会提示输入。

## 步骤

1. **读取指纹**

   ```powershell
   .\msboost-local-cli.exe fingerprint -host <IP>
   ```

   服务商控制台能看到 SSH 指纹的话，核对一致。

2. **部署**

   ```powershell
   .\msboost-local-cli.exe deploy -host <IP> -fingerprint <指纹>
   ```

   - 输入 root 密码（屏幕不显示），等几分钟。
   - 预期结果为 `succeeded`；`服务：running`，`本地自测：passed`。`公网 TCP` 测的是这台电脑到节点端口，`unreachable` 时检查服务商安全组。`BBR` 显示 `applied` 或 `unavailable` 都正常。
   - 当前目录生成 `直连-<IP>.json`。

3. **启动器验证**：把 `直连-<IP>.json` 放进启动器的 `configs` 文件夹，在「节点配置」里选中并开始加速。「网络检测」三项正常，并能进游戏。

4. **取回配置**

   ```powershell
   .\msboost-local-cli.exe fetch -host <IP> -fingerprint <指纹> -out 取回.json
   fc.exe /b "直连-<IP>.json" 取回.json
   ```

   预期 `fc` 显示两个文件没有差异。

5. **断线后取回（可选）**
   1. 再执行一次第 2 步。进度中断开电脑网络约 1 分钟，或者按 Ctrl+C。
   2. 恢复网络，等 5 分钟后执行第 4 步的 `fetch`。预期拿到新配置：用户名、密码、端口都和第 2 步不同。
   3. 用新配置在启动器里验证能连上。

6. **指纹不符（可选）**：把 `<指纹>` 的最后一个字符改掉再执行 `fetch`。预期提示「SSH 主机指纹已变化，操作已中止」，没有连接成功。

7. **卸载清理**

   ```powershell
   .\msboost-local-cli.exe cleanup -host <IP> -fingerprint <指纹>
   ```

   核对列出的路径后输入 `yes`，预期结果为 `succeeded`。再执行一次 `fetch`，应提示「这台服务器上没有找到 MSBOOST 节点」。

## 回报

把每一步的屏幕输出发回来。不要发 SSH 密码，也不要发配置文件内容：配置里有节点密码。
