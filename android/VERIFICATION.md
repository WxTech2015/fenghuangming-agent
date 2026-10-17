# Android 预览版验证记录

日期：2026-10-17。版本：`0.2.0-android.5`。

## 开发机检查

| 检查 | 范围 |
| --- | --- |
| Go 单元与协议测试 | 认证握手、指令 ACK、事件 ACK、租约、重复指令、旧任务、停止下载、制动持久化、日志脱敏、下载损坏与取消，以及安装和命令诊断回归 |
| Go 静态检查 | `go vet ./...` |
| Kotlin 单元测试 | 3 个状态机测试；初始制动、租约期限、旧 epoch、错误任务和整数秒范围 |
| WebUI | 类型检查、生产构建、2 个桥接编码测试；浏览器模拟同步 / 异步桥接、一秒跳转、进度条和制动 |
| Windows 回归 | 原 Agent 类型检查及 16 个现有测试；Windows 源码未改动 |
| APK | release 构建、APK v2 签名校验、包名和 minSdk 29 / targetSdk 36 检查 |
| ARM64 程序 | Android NDK 编译；AArch64 PIE，`/system/bin/linker64`，仅依赖系统 `liblog.so`、`libdl.so`、`libc.so` |
| 模块包 | Shell 语法、脚本 LF 换行、逐文件 SHA-256、ZIP 解压复核、私钥和配置排除 |

Go 单元测试在 Windows 和 GitHub Actions 的 Linux 上运行。Android 本机套接字、Root 私有缓冲写入、普通应用原生读取和真实播放已在下面列出的 MuMu 环境执行；ARM64 华为手机仍未连接。

AGP 的 `Instantiatable` 检查对本项目 Media3 服务产生继承链误报，仅在该服务声明上作了局部豁免。构建脚本用 `javap` 核对真实类文件：`PlaybackService → MediaSessionService → android.app.Service`，并检查公开无参构造函数。其他 Lint 检查保留。

## 安装问题修复

最初的手机日志中包管理器返回 `Failed transaction (2147483646)`，旧启动脚本随后退出。android.2 改用 Go 管道传输后，手机已进入包管理器安装逻辑，但末尾的 `-` 被当成未知选项，安装仍失败。

android.3 将命令修正为 `pm install -r --user 0 -S <字节数>`，省略 APK 路径，从标准输入读取。已核对 [Android 10](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/android10-release/services/core/java/com/android/server/pm/PackageManagerShellCommand.java) 与 [Android 14](https://android.googlesource.com/platform/frameworks/base/+/refs/heads/android14-release/services/core/java/com/android/server/pm/PackageManagerShellCommand.java) 的安装和标准输入处理代码。命令继续使用独立管道，不把 `/data/adb` 中的日志或 APK 文件描述符传给 Binder。安装前后继续校验 APK 和应用 UID；失败不阻止守护启动，重复安装由进程锁拦截。

补充检查覆盖流式安装参数、字节完整性、损坏 APK 拦截、具体安装错误保留、输出上限与进程超时，以及安装失败后仍启动守护的 Shell 回归。Linux 另检查命令子进程接收的三个标准流确实是管道。开发机完成 Windows 用例及 Android 交叉编译，Linux 用例由 GitHub Actions 执行；目标手机上的实际安装结果仍待更新后确认。

本次先按 Android 选项规则复现旧命令的 `Unknown option -`，修正后相同用例通过。该检查验证命令契约，不等同于手机实际安装。

## android.4 模拟器实测

环境为用户提供的 MuMu Android 15，ABI 为 x86_64，Go 守护使用 Android NDK 编译为该架构。APK 为同一 release 文件，包名 `com.phoring.fenghuangming.player`，版本代码 20004。模拟器原本为 SELinux Permissive，测试没有切换 SELinux 或修改其他模块。

旧版 android.3 在 MuMu 启动后服务仍在前台、崩溃缓冲为空；启动页代码主动调用 `finish()`，界面立即关闭。android.4 改为保留原生状态页，并记录服务创建、停止和启动错误；状态页会显示服务就绪、制动与错误状态。

| 实际执行 | 结果 |
| --- | --- |
| 模块安装检查入口 | 已安装 APK 的哈希和 UID 校验通过；模拟器中通过 ADB 安装 APK |
| 后端 WebSocket | 本机模拟后端鉴权、welcome 和租约通过 |
| 下载与原生读取 | 35 秒合成 WAV，4 路 Range 下载、哈希校验、Root 写入 APK 私有缓冲、原生解码通过 |
| 播放事件和进度 | 收到实际 `started` 事件，进度推进到 2.376 秒，时长 35 秒且可跳转 |
| 暂停与按秒跳转 | 暂停后跳转 7 秒，再增加到 8 秒，保持暂停；恢复后进度超过 9 秒 |
| 紧急制动 | 本次管理请求往返约 61 ms；随后验证制动状态、播放器不可接收新歌且无当前任务 |
| 重启播放服务 | 启动服务不会解除制动；手动解除后可再次连接并播放第二首 |
| 租约失效 | 停止续租后，实际播放任务停止 |
| 原生启动状态页 | 读取到服务写入的就绪状态 |
| 冷启动 | 连续 5 次强制停止后启动，每次前台服务正常；最后一次另观察 11 秒 |
| 崩溃缓冲 | 测试结束未出现凤凰鸣播放器崩溃 |

测试使用独立模拟后端和临时设备凭证，没有连接生产后台，没有向 QQ 群发送消息。以上证明模拟器中的播放链路可运行，不代表华为 ROM、蓝牙 / 耳机、开机和熄屏保活已经通过。模拟器扬声器音色未作人工听感验收。

## android.5 真实模块安装与重启

android.4 的上一节仅验证 APK 和临时目录中的守护程序，没有经过 KernelSU 模块安装，不能作为完整模块验收。android.5 补充以下实际操作，环境为同一 MuMu Android 15、x86_64、KernelSU 管理器 3.2.5（32525-2），SELinux 保持其原有 Permissive 状态。

测试模块来自 ARM64 发布 ZIP，替换 Go 守护为同源码编译的 Android x86_64 程序，调整 `customize.sh` 的架构检查与 `module.prop` 测试标记，再生成对应哈希清单。APK、WebUI、启动 / 停止脚本与发布包一致。测试包未作为 ARM64 发布包分发，也不代表 ARM64 二进制已在模拟器原生执行。

| 实际执行 | 结果 |
| --- | --- |
| 首次模块安装 | 通过 `/data/adb/ksud module install` 安装测试 ZIP，移除此前 ADB 手动安装的测试 APK，再重启 MuMu |
| 开机自动安装 APK | 模块开机脚本自行安装播放器；安装后 APK 哈希和 UID 校验通过 |
| 更新与重启 | 通过 KernelSU 安装修订后的测试 ZIP 并重启；模块、APK 和守护显示 android.5 |
| 重复开机钩子 | 实测发现两个监控脚本；改用 BusyBox 按锁文件执行监控，重启及再次调用 `service.sh` 后只保留一套监控与一个守护 |
| 真实 WebUI | 从 KernelSU 模块列表打开凤凰鸣页面，使用其真实 Java 桥接，页面读取到开机守护运行状态 |
| 连接配置 | 在页面输入本机测试后端地址与临时设备凭证，点击保存；守护认证成功，页面保留制动 |
| 原生服务 | 从页面点击启动播放器，完成模拟器通知授权；原生状态页显示服务运行，系统前台服务记录正常 |
| 实际播放 | 本机后端向已安装模块下发 60 秒低音量合成 WAV；收到 started，时长 60 秒且进度持续增加 |
| 页面播放控制 | 实际点击暂停、增加 1 秒、恢复及制动；分别检查守护和原生播放器状态，制动后无当前任务且不可接收歌曲 |
| 手动停止与恢复 | 页面停止守护后保持停用至少 5 秒；点击启动守护后重新认证成功 |
| 进程恢复 | 只终止经过可执行路径校验的本项目守护；监控重新启动守护并完成后端认证 |
| 配置与制动跨重启 | 再次重启 MuMu；地址、凭证保存标记和制动状态仍在，开机守护及单一监控正常 |

本轮使用 ADB 连接和本机模拟后端，没有接入 QQ 或生产后台。模拟器系统媒体音量为 0：已验证原生解码、播放状态和进度，未验证扬声器实际可听效果。没有测试华为 ARM64 真机、耳机 / 蓝牙、熄屏网络、长时间待命或模块卸载。Linux 增加并发监控和停用后重启的 Shell 回归；Windows 跳过该条 Linux 锁测试。

## 未完成的手机验收

开发机已在 MuMu 安装并重启测试模块，没有连接或修改目标华为手机。本版本为测试预览，以下真机行为仍需确认：

- SukiSU 管理器 WebUI 桥接和 APK 安装。
- SELinux 下 Root 与普通应用的 IPC、私有缓冲写入和原生读取。
- 真实音频输出、解码、音频焦点、暂停与一秒跳转。
- 下载中停止、制动、重启后保持锁定及断网租约。
- 开机、熄屏、长时间空闲、ROM 杀进程后的有限恢复。
- 模块禁用、更新和卸载，耳机 / 蓝牙切换。

首轮使用独立测试播放区与设备凭证，并保留可停用模块的恢复方式。发现失败时从 WebUI 导出日志，记录 Android API、ROM 和 SukiSU 版本。
