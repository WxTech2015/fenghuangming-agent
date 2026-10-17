# Android 预览版验证记录

日期：2026-10-17。版本：`0.2.0-android.2`。

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

Go 测试在 Windows 运行，验证平台无关的协议和逻辑；Android 本机套接字、进程锁及应用私有缓冲路径只完成交叉编译和代码检查，尚未在目标系统执行。

AGP 的 `Instantiatable` 检查对本项目 Media3 服务产生继承链误报，仅在该服务声明上作了局部豁免。构建脚本用 `javap` 核对真实类文件：`PlaybackService → MediaSessionService → android.app.Service`，并检查公开无参构造函数。其他 Lint 检查保留。

## android.2 安装修复

用户截图中的包管理器返回 `Failed transaction (2147483646)`，旧启动脚本随后退出。新安装流程通过 Go 创建独立管道，以 `pm install -S <字节数> -` 接收 APK，不把 `/data/adb` 中的日志或 APK 文件描述符传给 Binder。安装前核对模块 APK 的 SHA-256，安装后仍检查实际 APK 哈希和应用 UID；失败不阻止守护启动，重复安装由进程锁拦截。

补充检查覆盖流式安装参数、字节完整性、损坏 APK 拦截、具体安装错误保留、输出上限与进程超时，以及安装失败后仍启动守护的 Shell 回归。Linux 另检查命令子进程接收的三个标准流确实是管道。开发机完成 Windows 用例及 Android 交叉编译，Linux 用例由 GitHub Actions 执行；目标手机上的实际安装结果仍待更新后确认。

## 未完成的手机验收

开发机没有连接目标手机，没有安装 APK、模块或修改手机设置。本版本为测试预览，以下行为不能仅靠桌面检查确认：

- SukiSU 管理器 WebUI 桥接和 APK 安装。
- SELinux 下 Root 与普通应用的 IPC、私有缓冲写入和原生读取。
- 真实音频输出、解码、音频焦点、暂停与一秒跳转。
- 下载中停止、制动、重启后保持锁定及断网租约。
- 开机、熄屏、长时间空闲、ROM 杀进程后的有限恢复。
- 模块禁用、更新和卸载，耳机 / 蓝牙切换。

首轮使用独立测试播放区与设备凭证，并保留可停用模块的恢复方式。发现失败时从 WebUI 导出日志，记录 Android API、ROM 和 SukiSU 版本。
