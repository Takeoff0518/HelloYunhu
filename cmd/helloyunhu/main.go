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
	"github.com/Takeoff0518/HelloYunhu/internal/config"
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

const usageText = `用法: helloyunhu <命令> [参数]

命令:
  run       加载配置并启动转换进程
  version   输出版本信息
  selftest  检查运行环境与配置
  help      输出本说明

执行 helloyunhu <命令> -h 查看该命令的参数。
`

// 子命令共用的默认参数值。
const defaultConfigPath = "config.yaml"

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch 按子命令分发参数，返回进程退出码。
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return exitUsage
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
func runServe(args []string, _, stderr io.Writer) int {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := resolveBuildInfo()
	err := app.Run(ctx, app.Options{
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
	fmt.Fprintf(stdout, "Go: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stdout, "[失败] 配置检查: %v\n", err)
		fmt.Fprintln(stdout, "自检未通过")
		return exitError
	}

	fmt.Fprintf(stdout, "[通过] 配置检查: %s，共 %d 个实例\n", *configPath, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		fmt.Fprintf(stdout, "       实例 %s 连接 %s\n", instance.Name, instance.OneBot.URL)
	}

	fmt.Fprintln(stdout, "[跳过] 云湖接口连通性：等待 M1 实现")
	fmt.Fprintln(stdout, "[跳过] 事件订阅连接：等待 M1 实现")
	fmt.Fprintln(stdout, "[跳过] 框架握手：等待 M1 实现")
	fmt.Fprintln(stdout, "自检结束，跳过的项目没有得到验证")
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
