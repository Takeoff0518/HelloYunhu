// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfig = `
log:
  level: debug
  format: text
instances:
  - name: main
    token: token-value
    onebot:
      url: ws://127.0.0.1:6199/ws
`

// writeConfig 把配置写入临时目录并返回路径。
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

func TestRunMissingConfig(t *testing.T) {
	var logs bytes.Buffer

	err := Run(context.Background(), Options{
		ConfigPath: filepath.Join(t.TempDir(), "absent.yaml"),
		Version:    "test",
		LogOutput:  &logs,
	})
	if err == nil {
		t.Fatal("配置文件不存在时应返回错误")
	}
	if !strings.Contains(err.Error(), "读取配置文件") {
		t.Errorf("错误信息应说明读取失败，实际为: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("配置加载失败时不应产生日志，实际为: %q", logs.String())
	}
}

func TestRunInvalidConfig(t *testing.T) {
	path := writeConfig(t, "instances: []\n")
	var logs bytes.Buffer

	err := Run(context.Background(), Options{ConfigPath: path, LogOutput: &logs})
	if err == nil {
		t.Fatal("配置不合法时应返回错误")
	}
	if !strings.Contains(err.Error(), "至少需要配置一个机器人实例") {
		t.Errorf("错误信息应包含具体原因，实际为: %v", err)
	}
}

func TestRunUntilContextCancelled(t *testing.T) {
	path := writeConfig(t, testConfig)
	var logs bytes.Buffer

	// 先取消上下文，Run 记录启动信息后应立刻返回。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, Options{
		ConfigPath: path,
		Version:    "test",
		Commit:     "abcdef0",
		LogOutput:  &logs,
	})
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}

	output := logs.String()
	for _, want := range []string{"程序启动", "version=test", "commit=abcdef0", "装载机器人实例", "instance=main", "进程结束"} {
		if !strings.Contains(output, want) {
			t.Errorf("日志中缺少 %q，实际为:\n%s", want, output)
		}
	}
	if strings.Contains(output, "token-value") {
		t.Error("日志不应输出机器人令牌")
	}
}
