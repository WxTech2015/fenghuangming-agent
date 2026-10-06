# 参与开发

使用 Node.js 24 和 npm，在本仓库根目录执行：

```powershell
npm ci --include=dev
npm run typecheck
npm run build
npm test
```

设备联网上报与命令处理在 `src/service.ts`，用户会话播放与音频输出在 `src/worker.ts` 和 `src/mpv.ts`，音频缓存与下载在 `src/download.ts`，本机控制接口在 `src/control.ts`，桌面与服务安装脚本在 `scripts/`。

协议源文件由 [前后端仓库](https://github.com/WxTech2015/fenghuangming/blob/main/backend/src/contracts/index.ts) 维护。本仓库的 `src/contracts/index.ts` 是随源码提交的副本，运行和构建不在线获取协议。修改协议时同时更新两个仓库的副本，检查命令有效期、播放任务身份与新旧版本兼容性。

Windows x64 发行包在 Windows 的 Node.js 24+ 环境执行 `npm run package:win`，再运行 `npm run test:package`。打包工具获取经过校验的 mpv 和 NSSM；验证工具使用本机临时测试服务器，不安装系统服务。真实声卡切换、登录任务和目标后端连接需在部署环境验证。

提交问题时附版本、系统、操作步骤和脱敏日志。设备凭证、IPC 密钥、私人配置、日志和缓存不提交到仓库。项目源码采用 MIT 协议，贡献代码沿用该协议；第三方组件保留各自许可证。
