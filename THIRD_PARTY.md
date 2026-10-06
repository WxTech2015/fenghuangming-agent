# 第三方组件

凤凰鸣 Agent 自行编写的源码采用 [MIT](LICENSE)。依赖和发行包附带的软件保留各自许可证，具体版本以锁文件和 `scripts/package.cjs` 为准。

| 组件 | 用途 | 许可证 / 来源 |
| --- | --- | --- |
| [Node.js](https://github.com/nodejs/node) | 独立程序运行时 | MIT 及其附带的第三方许可证 |
| [mpv Windows 构建](https://github.com/shinchiro/mpv-winbuild-cmake) | 音频播放器 | 保留构建附带的许可证；播放器源码见 [mpv](https://github.com/mpv-player/mpv) |
| [NSSM](https://nssm.cc/) | Windows 服务管理 | 上游声明 Public Domain |
| [ws](https://github.com/websockets/ws)、[zod](https://github.com/colinhacks/zod) | WebSocket 和数据校验 | MIT |
| TypeScript、esbuild、postject、tsx、Vitest | 开发、测试和打包工具 | 以各 npm 包的许可证为准 |

源码仓库不包含运行时、播放器或服务管理器二进制。Windows 打包流程附带软件声明、版本、校验和及上游许可证；分发发行包时一并保留。
