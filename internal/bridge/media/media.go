// SPDX-License-Identifier: GPL-3.0-or-later

// Package media 负责读取与下载媒体内容。
//
// OneBot 的图片、文件与视频段可以用网络地址、base64 或本地路径给出，云湖要求先
// 上传取得标识，因此发送前需要把内容读成字节。接收方向的云湖媒体是网络地址，
// 需要下载后再交给框架，避免框架直接访问云湖 CDN 时受到来源限制。
package media

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// defaultMaxBytes 是单次读取的上限。图片、文件、视频与语音共用这一个上限。
const defaultMaxBytes = 20 << 20

// downloadTimeout 是下载超时时间。
const downloadTimeout = 60 * time.Second

// 支持的前缀。
const (
	base64Prefix = "base64://"
	filePrefix   = "file://"
)

// ErrTooLarge 表示媒体体积超过上限。
var ErrTooLarge = errors.New("媒体体积超过上限")

// Loader 读取媒体内容。
type Loader struct {
	// HTTPClient 为空时使用带超时的默认客户端。
	HTTPClient *http.Client
	// MaxBytes 是单次读取上限，0 表示使用默认值。
	MaxBytes int64
}

// Load 按来源读取媒体内容，返回文件名与字节。
//
// 支持 base64://、file://、http 或 https 地址，以及本地路径。来源无法识别时按
// 本地路径处理，由调用方给出的错误信息判断问题。
func (l Loader) Load(ctx context.Context, source string) (string, []byte, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil, errors.New("媒体来源为空")
	}

	switch {
	case strings.HasPrefix(source, base64Prefix):
		data, err := l.decodeBase64(source)
		if err != nil {
			return "", nil, err
		}
		return "upload.bin", data, nil

	case strings.HasPrefix(source, filePrefix):
		return l.readFile(localPath(strings.TrimPrefix(source, filePrefix)))

	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		return l.download(ctx, source)

	default:
		return l.readFile(source)
	}
}

// decodeBase64 解码 base64:// 形式的内容。
func (l Loader) decodeBase64(source string) ([]byte, error) {
	encoded := strings.TrimPrefix(source, base64Prefix)

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// 兼容不带填充的写法。
		data, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("解码 base64 媒体失败: %w", err)
		}
	}

	if int64(len(data)) > l.limit() {
		return nil, fmt.Errorf("%w: %d 字节", ErrTooLarge, len(data))
	}

	return data, nil
}

// localPath 把 file:// 后面的部分转换为本机路径。
//
// 框架常用 file:/// 加绝对路径的形式，例如 NoneBot2 给出的
// file:///E:/work/image.png。前三斜杠去掉之后剩下 /E:/work/image.png，在 Windows
// 上多了一个前导斜杠，读取会失败，这里按盘符判断并去掉它。
func localPath(raw string) string {
	if len(raw) >= 3 && raw[0] == '/' && raw[2] == ':' {
		letter := raw[1]
		if (letter >= 'a' && letter <= 'z') || (letter >= 'A' && letter <= 'Z') {
			return raw[1:]
		}
	}
	return raw
}

// readFile 读取本地文件。
func (l Loader) readFile(path string) (string, []byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("读取媒体文件 %s: %w", path, err)
	}
	if info.Size() > l.limit() {
		return "", nil, fmt.Errorf("%w: %d 字节", ErrTooLarge, info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("读取媒体文件 %s: %w", path, err)
	}

	return path, data, nil
}

// download 下载网络媒体。
func (l Loader) download(ctx context.Context, source string) (string, []byte, error) {
	client := l.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return "", nil, fmt.Errorf("构造媒体下载请求: %w", err)
	}

	// 云湖的图片 CDN 校验来源，缺少 Referer 时返回 403。社区适配器统一使用这个
	// 来源值，这里照同样的方式处理。
	request.Header.Set("User-Agent", "Mozilla/5.0")
	if referer := refererFor(source); referer != "" {
		request.Header.Set("Referer", referer)
	}

	response, err := client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("下载媒体 %s: %w", source, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("下载媒体 %s 返回状态码 %d", source, response.StatusCode)
	}

	// 多读一个字节用于判断是否超限。
	data, err := io.ReadAll(io.LimitReader(response.Body, l.limit()+1))
	if err != nil {
		return "", nil, fmt.Errorf("读取媒体内容: %w", err)
	}
	if int64(len(data)) > l.limit() {
		return "", nil, fmt.Errorf("%w: 超过 %d 字节", ErrTooLarge, l.limit())
	}

	return fileName(source), data, nil
}

// limit 返回生效的读取上限。
func (l Loader) limit() int64 {
	if l.MaxBytes > 0 {
		return l.MaxBytes
	}
	return defaultMaxBytes
}

// yunhuReferer 是云湖媒体 CDN 要求的来源地址。
const yunhuReferer = "https://myapp.jwznb.com"

// 需要对云湖 CDN 附带来源的域名后缀。
var yunhuHosts = []string{".jwznb.com", ".jwzhd.com"}

// refererFor 返回下载该地址时需要附带的来源地址，非云湖 CDN 返回空。
func refererFor(source string) string {
	parsed, err := url.Parse(source)
	if err != nil {
		return ""
	}

	host := parsed.Hostname()
	for _, suffix := range yunhuHosts {
		if strings.HasSuffix(host, suffix) {
			return yunhuReferer
		}
	}

	return ""
}

// fileName 从地址中提取文件名，取不到时给出默认值。
func fileName(source string) string {
	parsed, err := url.Parse(source)
	if err != nil {
		return "upload.bin"
	}

	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "upload.bin"
	}
	return name
}
