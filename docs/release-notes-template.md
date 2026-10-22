## MSBOOST 本地版 {{VERSION}}

**系统要求**：64 位 Windows 10 或 Windows 11。服务器为 Debian 11–13，root 能用密码或私钥登录，并能访问 GitHub。

### 下载与使用

1. 下载 `MSBOOST-Local-{{VERSION}}-win-x64.zip`，解压到 MSBOOST 启动器所在的文件夹（和 `MSBOOST-Launcher.exe` 放在一起），部署出的配置会直接出现在启动器里。
2. 双击 `MSBOOST-Local.exe`，不需要管理员权限。
3. 点「添加」，填写服务器的 IP 和 SSH 端口，核对并信任服务器指纹；选中服务器点「部署节点」，输入 root 密码。
4. 完成后点右上角的「打开启动器」，在启动器里选中新配置，开始加速。

### 安全提示

- **密码不离开这台电脑**：SSH 密码和私钥口令只在内存里使用，不经过 MSBOOST 网站，也不保存到硬盘。
- **SmartScreen**：本程序没有代码签名，Windows 可能提示「Windows 已保护你的电脑」。点「更多信息」，再点「仍要运行」。
- **杀毒软件误报**：少数杀毒软件可能误报没有签名的新程序。可以把程序文件夹加入白名单，或者向杀毒软件厂商提交误报。
- **校验文件**：
  - `SHA256SUMS.txt` 列出了两个 zip 的 SHA-256；
  - 解压后的 `SHA256SUMS.txt` 列出了每个文件的 SHA-256。

### 卸载

先在程序里卸载服务器上的节点和中转（想保留可以不卸），再删除 `MSBOOST-Local.exe` 和旁边的 `local` 文件夹。本程序不改系统设置，也不在别处留下文件。

### 更新内容

- （发布前填写）

### 开源许可

MSBOOST 本地版依据 GNU 通用公共许可证第 3 版（GPL-3.0）发布。

- 同版本源代码：`MSBOOST-Local-{{VERSION}}-source.zip`，或 https://github.com/mozziexwz/node_local
- 所用第三方组件的版权与许可：见 zip 中的 `THIRD_PARTY_NOTICES.txt`
