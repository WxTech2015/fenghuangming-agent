# Android 9 / EMUI 9 兼容版

适用于 ARM64、Android 9（API 28），并且 SukiSU 已正常运行的设备。模块不安装或修改 SukiSU、内核、引导或分区。

本兼容版与 Android 10+ 常规版独立打包；两者使用同一模块 ID、播放器包名和数据目录。覆盖安装可保留服务器地址、设备凭证及制动状态，不能同时运行两个版本。

安装兼容 ZIP 并重启，在模块 WebUI 配置后端。播放器自动安装并保持前台待命，保留 CPU / Wi-Fi 锁、进程恢复、紧急制动和一秒跳转。

## 构建

在 Agent 仓库根目录执行：

```text
node android/scripts/build-daemon.cjs --android9
node android/scripts/build-player.cjs --android9
node android/scripts/build-webui.cjs --android9
node android/scripts/package.cjs --android9
```

产物位于 `release/android/android9/`，Android 10+ 构建产物仍位于 `release/android/`。兼容版守护使用 NDK API 28 和 ARMv8.0，APK 最低 API 28，前台服务按系统版本选择接口。WebUI 语法以 Chromium 66 为目标，补齐旧 WebView 缺少的标准内置函数。

华为 Mate 9 真机行为仍需验证，具体实测环境及结果见 `VERIFICATION.md`。
