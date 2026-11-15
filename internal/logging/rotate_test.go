// SPDX-License-Identifier: GPL-3.0-or-later

package logging

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock 是可手动推进的时钟，用于模拟跨天。
//
// 后台整理协程会读取当前时间，测试协程随时可能推进它，因此读写都要加锁，
// 否则竞态检测会报数据竞争。
type clock struct {
	mu sync.Mutex
	at time.Time
}

// now 返回当前模拟时间。
func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.at
}

// moveTo 把模拟时间推到指定日期。
func (c *clock) moveTo(year int, month time.Month, day int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.at = time.Date(year, month, day, 9, 0, 0, 0, time.Local)
}

// reporter 把后台整理的错误写进测试日志，避免断言失败时看不到原因。
type reporter struct {
	t *testing.T
}

// Write 实现 io.Writer。
func (r reporter) Write(p []byte) (int, error) {
	r.t.Logf("整理日志: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}

// newWriter 创建一个使用模拟时钟的写日志实例。
func newWriter(t *testing.T, dir string, keepDays int, compress bool, c *clock) *RotatingWriter {
	t.Helper()

	writer, err := NewRotatingWriter(RotateOptions{
		Dir:      dir,
		KeepDays: keepDays,
		Compress: compress,
		Now:      c.now,
		Report:   reporter{t: t},
	})
	if err != nil {
		t.Fatalf("创建写日志实例失败: %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
	})
	return writer
}

// writeLine 写入一行日志。
func writeLine(t *testing.T, writer io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(writer, line+"\n"); err != nil {
		t.Fatalf("写入日志失败: %v", err)
	}
}

// readFile 读取文本文件。
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(data)
}

// readGzip 解压并读取文件。
func readGzip(t *testing.T, path string) string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("打开压缩包 %s 失败: %v", path, err)
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取压缩包 %s 失败: %v", path, err)
	}
	return string(data)
}

func TestWritesIntoDatedFile(t *testing.T) {
	dir := t.TempDir()
	c := &clock{}
	c.moveTo(2026, time.November, 15)

	writer := newWriter(t, dir, 30, true, c)
	writeLine(t, writer, "第一条")

	path := filepath.Join(dir, "2026-11-15.log")
	if got := readFile(t, path); !strings.Contains(got, "第一条") {
		t.Errorf("当天日志内容为 %q", got)
	}
}

func TestRotatesAndCompressesPreviousDay(t *testing.T) {
	dir := t.TempDir()
	c := &clock{}
	c.moveTo(2026, time.November, 15)

	writer := newWriter(t, dir, 30, true, c)
	writeLine(t, writer, "第一天")

	c.moveTo(2026, time.November, 16)
	writeLine(t, writer, "第二天")

	if err := writer.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "2026-11-15.log")); !os.IsNotExist(err) {
		t.Errorf("前一天的原始文件应被压缩后删除，stat 结果为 %v", err)
	}

	packed := filepath.Join(dir, "2026-11-15.log.gz")
	if got := readGzip(t, packed); !strings.Contains(got, "第一天") {
		t.Errorf("压缩包内容为 %q", got)
	}

	today := filepath.Join(dir, "2026-11-16.log")
	if got := readFile(t, today); !strings.Contains(got, "第二天") {
		t.Errorf("当天日志内容为 %q", got)
	}
}

func TestCompressDisabledKeepsPlainFile(t *testing.T) {
	dir := t.TempDir()
	c := &clock{}
	c.moveTo(2026, time.November, 15)

	writer := newWriter(t, dir, 30, false, c)
	writeLine(t, writer, "第一天")

	c.moveTo(2026, time.November, 16)
	writeLine(t, writer, "第二天")

	if err := writer.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	path := filepath.Join(dir, "2026-11-15.log")
	if got := readFile(t, path); !strings.Contains(got, "第一天") {
		t.Errorf("未开启压缩时旧文件内容为 %q", got)
	}
	if _, err := os.Stat(path + ".gz"); !os.IsNotExist(err) {
		t.Errorf("未开启压缩时不应生成压缩包，stat 结果为 %v", err)
	}
}

func TestTidyRemovesExpiredAndCompressesRemaining(t *testing.T) {
	dir := t.TempDir()
	expired := filepath.Join(dir, "2026-10-01.log")
	recent := filepath.Join(dir, "2026-11-10.log")
	for _, path := range []string{expired, recent} {
		if err := os.WriteFile(path, []byte("旧日志\n"), 0o644); err != nil {
			t.Fatalf("准备旧日志失败: %v", err)
		}
	}

	c := &clock{}
	c.moveTo(2026, time.November, 15)

	writer := newWriter(t, dir, 7, true, c)
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Errorf("超出保留天数的文件应被删除，stat 结果为 %v", err)
	}
	if got := readGzip(t, recent+".gz"); !strings.Contains(got, "旧日志") {
		t.Errorf("保留期内的压缩包内容为 %q", got)
	}
}

func TestNegativeKeepDaysKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "2020-01-01.log")
	if err := os.WriteFile(old, []byte("很久以前\n"), 0o644); err != nil {
		t.Fatalf("准备旧日志失败: %v", err)
	}

	c := &clock{}
	c.moveTo(2026, time.November, 15)

	writer := newWriter(t, dir, -1, false, c)
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	if got := readFile(t, old); !strings.Contains(got, "很久以前") {
		t.Errorf("保留天数为 -1 时不应删除旧文件，内容为 %q", got)
	}
}

func TestWritesAppendAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	c := &clock{}
	c.moveTo(2026, time.November, 15)

	first := newWriter(t, dir, 30, true, c)
	writeLine(t, first, "重启前")
	if err := first.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	second := newWriter(t, dir, 30, true, c)
	writeLine(t, second, "重启后")
	if err := second.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}

	got := readFile(t, filepath.Join(dir, "2026-11-15.log"))
	if !strings.Contains(got, "重启前") || !strings.Contains(got, "重启后") {
		t.Errorf("同一文件应追加写入，内容为 %q", got)
	}
}

func TestParseLogName(t *testing.T) {
	cases := []struct {
		name       string
		wantDay    string
		compressed bool
		ok         bool
	}{
		{name: "2026-11-15.log", wantDay: "2026-11-15", ok: true},
		{name: "2026-11-15.log.gz", wantDay: "2026-11-15", compressed: true, ok: true},
		{name: "helloyunhu.log", ok: false},
		{name: "2026-11-15.txt", ok: false},
		{name: "2026-13-01.log", ok: false},
	}

	for _, tc := range cases {
		day, compressed, ok := parseLogName(tc.name)
		if day != tc.wantDay || compressed != tc.compressed || ok != tc.ok {
			t.Errorf("%s 解析为 %q、%v、%v", tc.name, day, compressed, ok)
		}
	}
}
