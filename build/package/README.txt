MSBOOST 本地版 {{VERSION}}
==============================================================================

在自己的电脑上用 SSH 部署和管理 MSBOOST 节点与中转。SSH 密码和私钥口令
只在这台电脑的内存里使用，不经过 MSBOOST 网站，也不保存到硬盘。

使用
----
1. 把这些文件解压到 MSBOOST 启动器所在的文件夹（和 MSBOOST-Launcher.exe
   放在一起），部署出的配置会直接出现在启动器里。
2. 双击 MSBOOST-Local.exe，不需要管理员权限。
3. 点「添加」，填写服务器的 IP 和 SSH 端口，核对并信任服务器指纹。
4. 选中服务器点「部署节点」，输入 root 密码，等几分钟。
5. 点右上角的「打开启动器」，在启动器里选中新配置，开始加速。

「中转」页可以建自备中转（可加前置机），并管理服务器上的中转；
「朋友中转」页可以帮朋友建中转，或用朋友的中转生成新配置，双方只交换 IP 和端口。

程序的数据（服务器列表、任务记录、设置和日志）都在旁边的 local 文件夹里，
不含任何密码。不再使用时，先在程序里卸载服务器上的节点和中转（想保留可以不卸），
再删除 MSBOOST-Local.exe 和 local 文件夹。

文件
----
MSBOOST-Local.exe                      程序
MSBOOST-Local-README.txt               本说明
MSBOOST-Local-LICENSE.txt              GNU 通用公共许可证第 3 版（GPL-3.0）全文
MSBOOST-Local-THIRD_PARTY_NOTICES.txt  所用第三方组件的版权声明与许可
MSBOOST-Local-SHA256SUMS.txt           上面几个文件的 SHA-256

文件名都带「MSBOOST-Local」，解压到启动器的文件夹时不会覆盖启动器自己的
LICENSE.txt、THIRD_PARTY_NOTICES.txt 和 SHA256SUMS.txt。

源代码与许可
------------
MSBOOST 本地版依据 GNU 通用公共许可证第 3 版（GPL-3.0）发布，不提供任何担保。
同版本的完整源代码与本程序在同一下载位置提供：MSBOOST-Local-{{VERSION}}-source.zip
