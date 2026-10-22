# M3 打包与发布

M3 把程序做成可以发布的便携版：zip 里带上许可与说明文件，打标签时自动生成草稿 Release。下载和源码的做法与网站方案一致（网站仓库 `规划/MSBOOST本地版/MSBOOST-本地版下载与网站整改方案.md`，用户已确认）：

- 网站从自建网盘提供下载，网站上不出现 GitHub；
- GPL-3.0 第 6 条 d 款：同版本源码包和程序放在同一下载位置；
- zip 里的说明文件和程序的「开源许可」写明源码在哪里。

## 1. 完成内容

| 项目 | 内容 |
|---|---|
| 第三方声明 | `scripts/notices`（照启动器的做法）根据实际编进程序的 Go 依赖（`go list -deps`），加上 Go 本身和 React，生成 `THIRD_PARTY_NOTICES.txt`。相同的许可文本只写一次；文件开头说明 mihomo 和 GOST 由服务器自己下载，不在程序里 |
| 说明文件 | zip 里的 `README.txt`：使用步骤、文件清单、卸载方法，以及同版本源码包的文件名。模板在 `build/package/README.txt`，CI 填入版本号，换成 CRLF 行尾，方便记事本打开 |
| 开源许可对话框 | 加上源码位置（只有正式版显示，测试版不对外发布）和 `THIRD_PARTY_NOTICES.txt` 的说明；Go 扩展库去掉没有编进程序的 term |
| 发布包 | CI 产物改名为 `MSBOOST-Local-<版本>-win-x64`，与网站的文件名一致。内含 `MSBOOST-Local.exe`、`README.txt`、`LICENSE.txt`、`THIRD_PARTY_NOTICES.txt` 和 `SHA256SUMS.txt` |
| 正式发布 | 推送 `v*` 标签时，CI 编出带版本号的 exe（文件属性里的版本也会更新），等两个测试任务都通过后创建**草稿** Release，附上三个文件：`MSBOOST-Local-<版本>-win-x64.zip`、`MSBOOST-Local-<版本>-source.zip`（`git archive` 打包的同版本源码）和 `SHA256SUMS.txt`（两个 zip 的 SHA-256）。说明取自 `docs/releases/<版本>.md`，没有就用 [发布说明模板](../release-notes-template.md) |
| 发布说明 | [v1.0.0](../releases/v1.0.0.md) |

## 2. 实测步骤

### 准备

在仓库的 Actions 页面打开 `m3-release` 分支最新一次通过的 `ci` 运行，下载 `MSBOOST-Local-dev-xxxxxxx-win-x64`，解压到一个新文件夹。

### 测试 1：发布包

1. 文件夹里只有 `MSBOOST-Local.exe`、`README.txt`、`LICENSE.txt`、`THIRD_PARTY_NOTICES.txt` 和 `SHA256SUMS.txt`。
2. 用记事本打开 `README.txt` 和 `THIRD_PARTY_NOTICES.txt`：中文正常，分行正常。
3. 在这个文件夹里打开命令提示符，执行下面的命令，结果应与 `SHA256SUMS.txt` 里 `MSBOOST-Local.exe` 那一行相同：

   ```bat
   certutil -hashfile MSBOOST-Local.exe SHA256
   ```

### 测试 2：开源许可

打开程序，在「设置与关于」点「开源许可…」：
- 应有组件表、服务器端组件，以及「全部第三方组件的版权声明与许可全文见程序旁边的 THIRD_PARTY_NOTICES.txt」。
- 测试版不显示源码包那一句，正式版才显示。

### 测试 3（可选）：干净的电脑

在 Windows 10 虚拟机或另一台没用过本地版的电脑上，用普通用户账户：
- 解压即用，不需要管理员权限；
- 添加一台服务器并刷新状态；
- `local\` 里的文件找不到任何密码。

### 回报

把每项测试的结果发过来，有问题时附上截图。

## 3. 发布 v1.0.0

M3 的 PR 合并后，等你说发布再打标签。之后的步骤：

1. 我在 main 上打标签 `v1.0.0` 并推送，CI 生成草稿 Release（三个文件）。
2. 你用 Chrome 下载草稿里的 zip：
   - 下载不被拦截；
   - Windows 安全中心扫描 zip 和解压后的 exe，没有报毒；
   - 运行后「设置与关于」里版本显示 `v1.0.0`，「开源许可」里有源码包那一句。
3. 把三个文件上传到网盘 `MSBOOST本地版/v1.0.0/`。
4. 在网站「系统设置 → MSBOOST 本地版」填写版本 `v1.0.0`、下载地址和 zip 的 SHA-256（在 `SHA256SUMS.txt` 里）。
5. 在 GitHub 上发布草稿 Release。
