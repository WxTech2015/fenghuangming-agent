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
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *stop {
		if err := stopPlayer(); err != nil {
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
