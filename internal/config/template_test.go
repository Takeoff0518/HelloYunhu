// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWriteTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := WriteTemplate(path); err != nil {
		t.Fatalf("写入模板失败: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取模板失败: %v", err)
	}

	// 模板必须是结构合法的 YAML，具体配置项在用户填写后才校验。
	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("模板不是合法 YAML: %v", err)
	}
	for _, key := range []string{"bot", "receive", "onebot", "log", "storage", "yunhu", "capabilities"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("模板缺少配置段 %q", key)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取文件信息失败: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("文件权限为 %o，期望 600", info.Mode().Perm())
	}
}

func TestWriteTemplateRefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := WriteTemplate(path); err != nil {
		t.Fatalf("第一次写入失败: %v", err)
	}
	err := WriteTemplate(path)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("第二次写入返回 %v，期望 ErrExists", err)
	}
}
