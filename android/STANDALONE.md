# Android 9 独立 SH 安装

适用于已能获取 Root 的 Android 9+ ARM64 设备，使用 SukiSU / KernelSU 自带 BusyBox。无需写入 `/system`，不依赖模块 ZIP 的更新应用流程。原生播放器按普通 APK 安装。

下载 Release 中的 `.sh` 文件到手机，在终端执行：

```sh
su
sh /sdcard/Download/fenghuangming-agent-android9-arm64-0.2.0-android9.2.sh
sh /data/adb/fenghuangming/standalone/fhm.sh open
```

安装后立即启动，无需重启；`open` 使用手机浏览器打开本机控制台，在「设置」填写后端根地址和设备凭证，保存后手动解除制动。若浏览器无法自动打开，执行 `sh /data/adb/fenghuangming/standalone/fhm.sh url`，将输出的完整地址粘贴到本机浏览器。地址中含本机管理密钥，不要分享。

控制台只监听 `127.0.0.1:17789`，可查看播放、队列、下载、保活状态与日志，控制暂停、按秒跳转和紧急制动。控制台关闭后服务继续运行；网页点击停止只停播放守护，控制台保留，便于再次启动。

程序位于 `/data/adb/fenghuangming/standalone`，配置、缓存、日志保留在 `/data/adb/fenghuangming`。覆盖安装保留配置和制动状态，会停用本项目原模块，避免同时运行两个实例。安装不会删除旧模块的待更新信息，也不会更改其他模块。

开机入口为 `/data/adb/service.d/fenghuangming-standalone.sh`。它仍需 Root 框架在开机时执行 `service.d`；如果本机 SukiSU 的开机脚本机制也未工作，重启后需手动执行 `start`，不能仅靠一个 SH 文件保证自动启动。不会为此改写系统分区或内核。

```sh
sh /data/adb/fenghuangming/standalone/fhm.sh status
sh /data/adb/fenghuangming/standalone/fhm.sh start
sh /data/adb/fenghuangming/standalone/fhm.sh stop
sh /data/adb/fenghuangming/standalone/fhm.sh brake
sh /data/adb/fenghuangming/standalone/fhm.sh release
sh /data/adb/fenghuangming/standalone/fhm.sh logs
sh /data/adb/fenghuangming/standalone/fhm.sh shutdown
sh /data/adb/fenghuangming/standalone/fhm.sh uninstall
```

`shutdown` 停止守护与控制台。`uninstall` 另移除本项目创建的开机脚本和本项目新增的播放器待机白名单，保留程序、配置及 APK；需要时通过系统应用管理卸载播放器。

构建时先完成 Android 9 守护、APK、WebUI 和模块包，再执行 `node android/scripts/package-standalone.cjs --android9`。测试环境及尚未实测的范围见 `VERIFICATION.md`。
