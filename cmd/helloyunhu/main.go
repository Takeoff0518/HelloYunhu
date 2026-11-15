// SPDX-License-Identifier: GPL-3.0-or-later

// Command helloyunhu 在云湖机器人开放平台与 OneBot v11 协议之间做双向转换，
// 使基于 OneBot v11 编写的机器人框架可以直接接入云湖。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"github.com/Takeoff0518/HelloYunhu/internal/app"
)

// 版本信息在构建时通过链接参数写入，未注入时保留下面的默认值。
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// 进程退出码。
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const usageText = `用法: helloyunhu [命令] [参数]

不带参数时等同于 run，直接加载配置并启动。

命令:
  run       加载配置并启动转换进程
  version   输出版本信息
  selftest  检查运行环境与配置
  help      输出本说明

执行 helloyunhu <命令> -h 查看该命令的参数。首次运行时会生成 config.yaml，
填写机器人令牌与框架地址后再次运行即可。
`

// 子命令共用的默认参数值。
const defaultConfigPath = "config.yaml"

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch 按子命令分发参数，返回进程退出码。
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		// 与 go-cqhttp 一类程序的习惯一致：不带参数直接启动。
		return runServe(nil, stdout, stderr)
	}

	switch args[0] {
	case "run":
		return runServe(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "selftest":
		return runSelftest(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usageText)
		return exitOK
	default:
		fmt.Fprintf(stderr, "错误: 未知命令 %q\n\n%s", args[0], usageText)
		return exitUsage
	}
}

// runServe 加载配置并启动转换进程，收到中断信号后退出。
//
// 配置文件不存在时先生成模板并提示用户填写，然后退出；第二次运行才真正启动。
func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件路径")

	code, ok := parseFlags(fs, args, stderr)
	if !ok {
		return code
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "错误: run 不接受多余参数 %q\n", fs.Arg(0))
		return exitUsage
	}

	created, err := app.EnsureConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "错误: %v\n", err)
		return exitError
	}
	if created {
		fmt.Fprintf(stdout, "已生成配置文件 %s，本次不启动。\n", *configPath)
		fmt.Fprint(stdout, "请填写以下两项后再次运行：\n")
		fmt.Fprint(stdout, "  instances[0].token        云湖机器人令牌\n")
		fmt.Fprint(stdout, "  instances[0].onebot.url   机器人框架的 WebSocket 地址\n")
		return exitOK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := resolveBuildInfo()
	err = app.Run(ctx, app.Options{
		ConfigPath: *configPath,
		Version:    info.Version,
		Commit:     info.Commit,
		LogOutput:  stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "错误: %v\n", err)
		return exitError
	}

	return exitOK
}

// runVersion 输出版本信息。
func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)

	code, ok := parseFlags(fs, args, stderr)
	if !ok {
		return code
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "错误: version 不接受多余参数 %q\n", fs.Arg(0))
		return exitUsage
	}

	info := resolveBuildInfo()
	fmt.Fprintf(stdout, "helloyunhu %s\n", info.Version)
	fmt.Fprintf(stdout, "提交: %s\n", info.Commit)
	fmt.Fprintf(stdout, "构建时间: %s\n", info.Date)
	fmt.Fprintf(stdout, "Go: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return exitOK
}

// runSelftest 检查运行环境与配置。
//
// 目前只验证配置能否加载与校验。与云湖和框架的连通性检查依赖 M1 的连接实现，
// 在那之前明确列为跳过项，不计入通过。
func runSelftest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("selftest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", defaultConfigPath, "配置文件路径")

	code, ok := parseFlags(fs, args, stderr)
	if !ok {
		return code
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "错误: selftest 不接受多余参数 %q\n", fs.Arg(0))
		return exitUsage
	}

	info := resolveBuildInfo()
	fmt.Fprintf(stdout, "helloyunhu %s（提交 %s）\n", info.Version, info.Commit)
	fmt.Fprintf(stdout, "Go: %s %s/%s\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	results := app.Selftest(context.Background(), app.Options{
		ConfigPath: *configPath,
		Version:    info.Version,
		Commit:     info.Commit,
	})

	counts := make(map[string]int, 3)
	for _, result := range results {
		counts[result.Status]++
		fmt.Fprintf(stdout, "[%s] %s: %s\n", result.Status, result.Name, result.Detail)
	}

	fmt.Fprintf(stdout, "\n自检结束：%d 项通过，%d 项失败，%d 项跳过\n",
		counts[app.StatusPassed], counts[app.StatusFailed], counts[app.StatusSkipped])

	if counts[app.StatusFailed] > 0 {
		return exitError
	}
	if counts[app.StatusSkipped] > 0 {
		fmt.Fprintln(stdout, "跳过的项没有得到验证")
	}

	return exitOK
}

// parseFlags 解析子命令参数。参数解析失败时返回退出码，并把 ok 置为 false。
func parseFlags(fs *flag.FlagSet, args []string, _ io.Writer) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		return exitUsage, false
	}
	return exitOK, true
}

// buildInfo 描述程序版本，链接参数注入的值优先于构建信息中记录的值。
type buildInfo struct {
	Version string
	Commit  string
	Date    string
}

// resolveBuildInfo 读取链接参数与构建信息，得到最终的版本信息。
func resolveBuildInfo() buildInfo {
	info := buildInfo{Version: version, Commit: commit, Date: date}

	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	if info.Version == "dev" && build.Main.Version != "" && build.Main.Version != "(devel)" {
		info.Version = build.Main.Version
	}

	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "none" && len(setting.Value) >= 7 {
				info.Commit = setting.Value[:7]
			}
		case "vcs.time":
			if info.Date == "unknown" {
				info.Date = setting.Value
			}
		}
	}

	return info
}
