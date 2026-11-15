// SPDX-License-Identifier: GPL-3.0-or-later

package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 日志文件的命名与切分规则。
const (
	dateLayout = "2006-01-02"
	logSuffix  = ".log"
	gzipSuffix = ".log.gz"
	dirPerm    = 0o755
	filePerm   = 0o644
)

// RotateOptions 描述按日切分日志的选项。
type RotateOptions struct {
	// Dir 是日志目录，不存在时自动创建。
	Dir string
	// KeepDays 是保留天数，-1 表示永不删除，0 表示不清理（由调用方校验，不推荐）。
	KeepDays int
	// Compress 为真时把前一天的文件压缩成 gzip。
	Compress bool
	// Now 返回当前时间，为空时使用 time.Now。测试通过它模拟跨天。
	Now func() time.Time
	// Report 接收压缩与清理过程中的错误，为空时写到标准错误。
	//
	// 这些操作在后台进行，错误不影响日志写入，因此单独报告。
	Report io.Writer
}

// RotatingWriter 按天切分日志文件，写入失败以外的错误都不影响正常输出。
//
// 切分在写入时进行：写入前比较当天日期与当前文件日期，不同就关闭旧文件并打开
// 新文件，不需要定时器。旧文件的压缩与过期清理在后台协程里完成，不阻塞写入。
type RotatingWriter struct {
	dir      string
	keepDays int
	compress bool
	now      func() time.Time
	report   io.Writer

	mu   sync.Mutex
	file *os.File
	day  string

	// tidyMu 保证同一时刻只有一次整理在跑，避免两次整理抢同一个文件。
	tidyMu sync.Mutex
	// 后台任务，Close 时等待它们结束。
	workers sync.WaitGroup
}

// NewRotatingWriter 打开当天的日志文件，并触发一次旧文件整理。
//
// 目录创建或文件打开失败时返回错误，调用方应当据此终止启动，而不是在运行中
// 静默丢弃日志。
func NewRotatingWriter(opt RotateOptions) (*RotatingWriter, error) {
	if strings.TrimSpace(opt.Dir) == "" {
		return nil, fmt.Errorf("日志目录不能为空")
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Report == nil {
		opt.Report = os.Stderr
	}

	if err := os.MkdirAll(opt.Dir, dirPerm); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s: %w", opt.Dir, err)
	}

	writer := &RotatingWriter{
		dir:      opt.Dir,
		keepDays: opt.KeepDays,
		compress: opt.Compress,
		now:      opt.Now,
		report:   opt.Report,
	}

	day := writer.today()
	file, err := writer.open(day)
	if err != nil {
		return nil, err
	}
	writer.file = file
	writer.day = day

	// 启动时整理一次，补压上次进程退出前没来得及压缩的文件。
	writer.background()

	return writer, nil
}

// Write 写入一段日志，跨天时先切换到新文件。
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := w.today()
	if w.file == nil || w.day != day {
		file, err := w.open(day)
		if err != nil {
			// 新文件打不开时继续写旧文件，至少不丢日志。
			if w.file == nil {
				return 0, err
			}
		} else {
			if w.file != nil {
				_ = w.file.Close()
			}
			w.file = file
			w.day = day
			w.background()
		}
	}

	return w.file.Write(p)
}

// Close 等待后台整理结束并关闭当前文件。
//
// 先拿住写锁再等待，避免等待期间又有写入把新的整理任务加进等待组。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.workers.Wait()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// today 返回当前日期，格式为 2026-11-15。
func (w *RotatingWriter) today() string {
	return w.now().Format(dateLayout)
}

// open 打开指定日期的日志文件，以追加方式写入。
func (w *RotatingWriter) open(day string) (*os.File, error) {
	path := filepath.Join(w.dir, day+logSuffix)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePerm)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 %s: %w", path, err)
	}
	return file, nil
}

// background 启动一次旧文件整理。
func (w *RotatingWriter) background() {
	w.workers.Add(1)
	go func() {
		defer w.workers.Done()

		if err := w.tidy(); err != nil {
			fmt.Fprintf(w.report, "整理日志文件失败: %v\n", err)
		}
	}()
}

// tidy 压缩过期以外的旧文件，并删除超出保留天数的文件。
//
// 只处理文件名符合日期加 .log 或 .log.gz 的文件，目录里的其它内容不动。当天与
// 日期更晚的文件一律跳过：整理在后台跑，期间可能已经跨天并写出更新的文件，按
// 启动时拿到的日期判断会把新文件误当成旧文件压缩掉。
func (w *RotatingWriter) tidy() error {
	w.tidyMu.Lock()
	defer w.tidyMu.Unlock()

	today := w.today()

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("读取日志目录 %s: %w", w.dir, err)
	}

	cutoff := ""
	if w.keepDays > 0 {
		base, err := time.Parse(dateLayout, today)
		if err != nil {
			return fmt.Errorf("解析当天日期 %q: %w", today, err)
		}
		cutoff = base.AddDate(0, 0, -w.keepDays).Format(dateLayout)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		day, compressed, ok := parseLogName(name)
		if !ok || day >= today {
			continue
		}

		path := filepath.Join(w.dir, name)

		if cutoff != "" && day < cutoff {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("删除过期日志 %s: %w", path, err)
			}
			continue
		}

		if w.compress && !compressed {
			if err := gzipFile(path); err != nil {
				return err
			}
		}
	}

	return nil
}

// parseLogName 从文件名解析日期，并判断是否已压缩。
func parseLogName(name string) (day string, compressed bool, ok bool) {
	switch {
	case strings.HasSuffix(name, gzipSuffix):
		day = strings.TrimSuffix(name, gzipSuffix)
		compressed = true
	case strings.HasSuffix(name, logSuffix):
		day = strings.TrimSuffix(name, logSuffix)
	default:
		return "", false, false
	}

	if _, err := time.Parse(dateLayout, day); err != nil {
		return "", false, false
	}
	return day, compressed, true
}

// gzipFile 把日志文件压缩为同名 .gz，成功后删除原文件。
//
// 先关闭源文件再删除，否则 Windows 上会因文件仍被占用而拒绝删除。压缩中途失败时
// 删掉写了一半的 .gz，保留原文件，下次整理会重试。
func gzipFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开待压缩日志 %s: %w", path, err)
	}

	target := path + ".gz"
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
	if err != nil {
		_ = src.Close()
		return fmt.Errorf("创建压缩文件 %s: %w", target, err)
	}

	zipper := gzip.NewWriter(dst)
	_, copyErr := io.Copy(zipper, src)
	zipErr := zipper.Close()
	dstErr := dst.Close()
	srcErr := src.Close()

	if copyErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("压缩日志 %s: %w", path, copyErr)
	}
	if zipErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("结束压缩 %s: %w", path, zipErr)
	}
	if dstErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("关闭压缩文件 %s: %w", target, dstErr)
	}
	if srcErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("关闭源文件 %s: %w", path, srcErr)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除已压缩的日志 %s: %w", path, err)
	}
	return nil
}
