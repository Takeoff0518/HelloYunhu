// SPDX-License-Identifier: GPL-3.0-or-later

// Command helloyunhu 在云湖机器人开放平台与 OneBot v11 协议之间做双向转换，
// 使基于 OneBot v11 编写的机器人框架可以直接接入云湖。
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
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

// runServe 启动转换进程。
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

	fmt.Fprintf(stderr, "错误: 配置读取尚未实现，%s 没有被加载\n", *configPath)
	return exitError
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
func runSelftest(args []string, _, stderr io.Writer) int {
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

	fmt.Fprintf(stderr, "错误: 配置检查尚未实现，%s 没有被读取\n", *configPath)
	return exitError
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
