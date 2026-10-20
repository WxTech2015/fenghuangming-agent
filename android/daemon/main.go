package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	data := flag.String("data", defaultData, "数据目录")
	module := flag.String("module", "", "模块目录")
	request := flag.String("request", "", "Base64 管理请求")
	showVersion := flag.Bool("version", false, "版本")
	install := flag.Bool("install-player", false, "检查或安装本模块的播放 APK")
	stop := flag.Bool("stop-player", false, "停止本模块的播放应用")
	release := flag.Bool("release-keepalive", false, "移除本模块添加的播放器待机豁免")
	console := flag.Bool("console", false, "运行仅本机可访问的独立网页控制台")
	consoleURL := flag.Bool("console-url", false, "显示独立控制台登录地址")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *console || *consoleURL {
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "独立控制台需要 Root")
			os.Exit(1)
		}
		key, err := consoleKey(*data)
		if err == nil && *consoleURL {
			fmt.Println(consoleOrigin + "/#key=" + key)
			return
		}
		if err == nil && *module == "" {
			err = fmt.Errorf("缺少程序目录")
		}
		if err == nil {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			err = serveConsole(ctx, *data, *module, key)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	if *stop {
		if err := stopPlayer(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	if *release {
		if err := releaseKeepalive(*data); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	if *request != "" {
		if err := controlCLI(*data, *request); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	if *module == "" {
		fmt.Fprintln(os.Stderr, "缺少模块目录")
		os.Exit(1)
	}
	if *install {
		if err := installPlayer(*data, *module); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	unlock, err := lockDaemon(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	defer unlock()
	agent, err := newAgent(*data, *module)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	agent.cancelMain = cancel
	if err = serveControl(ctx, agent); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
