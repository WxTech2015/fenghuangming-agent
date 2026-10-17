# 凤凰鸣 Android Agent

适用于 SukiSU / KernelSU 的 Android 10+ ARM64 播放端，包含 Go 守护程序、Android Media3 原生播放器与 Vue 模块 WebUI。当前为 `0.2.0-android.5` 预览版，尚未完成华为 Mate 实机验证。

## 安装和首次连接

在 SukiSU 管理器中安装 `fenghuangming-agent-sukisu-arm64-0.2.0-android.5.zip` 并重启。模块等待系统就绪后安装自己的播放 APK、启动守护进程。打开模块 WebUI 的「设置」，填写凤凰鸣后端根地址和后台注册设备时生成的凭证，保存后手动解除制动。

首次验证使用独立测试播放区和设备，不复用正在运行的 Windows Agent 凭证。缓存默认 512 MB，播放器另有一个最多 200 MB 的单曲私有缓冲。每次写入前保留至少 256 MB 空闲空间；超过缓存上限的歌曲明确拒绝。

第一次可点击「启动播放器」，完成 Android 通知授权。守护、后端连接与播放器就绪分别显示；只有播放器真正就绪才接收歌曲。播放通过 Android 默认媒体输出，WebUI 关闭后继续运行。音量控制只调整本播放器的相对音量，手机系统媒体音量需单独设置。

`android.5` 修复开机钩子重复执行时出现多套监控的问题，使用 KernelSU 自带 BusyBox 的锁文件保证单一监控。已在 MuMu Android 15 的 KernelSU 3.2.5 中真正安装 x86_64 测试模块并重启，验证开机自动安装 / 更新 APK、启动守护、WebUI 保存连接配置、启动播放器、暂停 / 恢复、增加 1 秒与紧急制动。手动停止后保持停用，手动启动可恢复，守护退出后可自动恢复；再次重启会保留配置和制动状态。详细范围见 [验证记录](VERIFICATION.md)。

`android.4` 保留原生启动状态页，显示服务是否运行、制动状态及启动错误；点击「关闭此页面」只关闭状态页，播放服务继续运行。旧版启动页会主动关闭，容易被误认为闪退。本版已在 MuMu Android 15 验证 5 次冷启动及完整播放、暂停、按秒跳转、制动和租约停止；模拟器使用其原有 Permissive 状态，华为 Mate 真机与 Enforcing 权限兼容仍待确认。

`android.3` 修正安装命令末尾的 `-` 被系统当作未知选项的问题。APK 仍使用管道流式安装，命令提供字节数并省略 APK 路径，由包管理器从标准输入读取。安装失败时守护仍会运行，可在「设置 → 检查 / 安装播放组件」重试，再到「日志」查看结果；安装在后台执行，系统安装命令最多等待 90 秒，前后另做校验。日志按时间和版本标记每次安装尝试，便于区分历史错误。更新模块保留原配置和制动状态。仍失败时请导出完整日志，并提供 Android、ROM 和 SukiSU 版本。

如果 ROM 限制自动安装，可下载同一 Release 中的 `fenghuangming-player-0.2.0-android.5.apk`，通过手机系统安装器手动安装，再回到模块 WebUI 启动播放器。APK 与模块内的文件一致，仍须安装对应版本的模块。

WebUI 提供日志、下载进度、当前歌曲、待播队列、暂停 / 继续 / 停止、整数秒进度条、−1 秒 / +1 秒及端点配置。紧急制动先锁定状态再停止播放，重启保持锁定，必须在本机解除。配置变更后自动保持制动，不重播旧任务。

模块只管理自己的普通应用播放组件和守护进程，不写引导或分区，不安装 KPM，不注入系统进程，不关闭 SELinux 或修改全局电池策略。前台服务采用待命用途和实际媒体播放用途，系统是否允许后台启动、熄屏网络及音频输出仍以目标 ROM 实测为准。服务反复启动失败会停止自动恢复。

## 停用与卸载

在 WebUI 点击「停止服务」可停止本项目守护与播放应用；手动停止优先于自动保活。SukiSU 模块的禁用标记也会被守护检查。模块卸载只停止本项目，不删除其他应用、用户存储或配置；播放 APK 和 `/data/adb/fenghuangming` 数据保留，可通过系统应用管理单独卸载播放 APK。

日志和配置位于 `/data/adb/fenghuangming`，根目录仅 Root 可访问，设备凭证不返回页面。播放器与守护使用经过 UID 验证的 Unix 套接字通信。APK 创建自己的私有音频缓冲，守护核对身份后写入并校验，APK 自行只读打开；保留应用原有 SELinux 标签。播放器没有网络权限。

## 构建

需要 Node.js 24+、Go 1.24+、JDK 21、Gradle 8.13、Android NDK r27d、Android SDK API 36 扩展 19 和 Build Tools 36.1.0。Windows 可使用项目内隔离工具：

```powershell
node android/scripts/bootstrap.cjs
node android/scripts/prepare-ndk.cjs
node android/scripts/build-daemon.cjs
node android/scripts/build-player.cjs
```

APK 签名私钥与密码位于 `android/.signing/`，务必备份，不要上传 GitHub。后续更新须使用同一私钥。构建脚本不连接或安装到手机。

Go 程序在 `android/daemon` 运行 `go test ./...`。发行构建使用 `GOOS=android GOARCH=arm64 CGO_ENABLED=1`、NDK 的 API 29 编译器及 PIE 模式，输出到 `release/android/fhm-agent`。程序使用手机自带的 Android 链接器和 Bionic，域名解析交给系统，不使用普通 Linux 静态二进制的 DNS 假设。

在 `android/webui` 执行 `npm ci --include=dev`、`npm run build` 和 `npm test`，之后从 Agent 仓库根目录执行：

```powershell
node android/scripts/package.cjs
```

输出在 `release/android/`，ZIP 内包含哈希清单和开源声明，不包含私钥、配置、日志或缓存。Windows Agent 的构建和版本保持独立。

## 模拟器回归

`android/tests/emulator-smoke.cjs` 仅用于有 Root 的隔离 x86_64 Android 模拟器。先安装本次构建的 APK 并授权通知，再提供同架构的 Android Go 守护程序：

```text
node android/tests/emulator-smoke.cjs --serial <ADB设备地址> --daemon <Android x86_64守护程序> --adb <adb程序路径>
```

守护程序使用 `GOOS=android GOARCH=amd64 CGO_ENABLED=1`、NDK 的 `x86_64-linux-android29` 编译器及 PIE 模式构建，不能使用普通 Windows 或 Linux 程序。测试生成低音量 WAV、启动本机模拟后端并通过 ADB reverse 连接，临时数据位于 `/data/local/tmp/fhm-emulator-smoke-*`。该测试会启动和停止凤凰鸣播放应用，不读取正式模块配置。报告写入 `android/.tools/`，不要将报告中的环境日志上传公开仓库。

## 验收范围

安装前先确认完整备份与可停用模块的方式。首轮测试覆盖：开机后启动、WebUI 桥接、APK 原生播放、1 秒跳转、制动持久化、断网租约、耳机 / 蓝牙断开、熄屏与长时间待命。当前构建检查不代表上述真机行为已经验证，不应直接用于无人值守场景。检查范围见 [验证记录](VERIFICATION.md)。
