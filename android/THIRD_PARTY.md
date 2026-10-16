# Android 组件声明

凤凰鸣自行编写代码使用仓库 MIT 协议。以下依赖保留其许可证，准确版本见 Go、Gradle 和 npm 锁定配置。

| 组件 | 用途 | 许可证 |
| --- | --- | --- |
| Go 标准库 | 原生 ARM64 守护程序 | BSD-3-Clause |
| gorilla/websocket 1.5.3 | 后端 WebSocket 协议 | BSD-2-Clause |
| AndroidX Media3 1.8.1 | 原生媒体播放与媒体会话 | Apache-2.0 |
| Kotlin 标准库 | 播放 APK | Apache-2.0 |
| Vue、Naive UI | 模块管理页面 | MIT |

Android SDK、JDK、Gradle、Node.js 和 Go 编译器仅用于构建，不将其安装包分发到手机。模块不包含 Windows mpv、NSSM 或 Node.js 运行时。

上游来源：

- https://go.dev/LICENSE
- https://github.com/gorilla/websocket/blob/v1.5.3/LICENSE
- https://github.com/androidx/media/blob/1.8.1/LICENSE
- https://github.com/JetBrains/kotlin/blob/master/license/LICENSE.txt
- https://github.com/vuejs/core/blob/main/LICENSE
- https://github.com/tusen-ai/naive-ui/blob/main/LICENSE
