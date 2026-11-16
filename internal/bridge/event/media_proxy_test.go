// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/proxypath"
)

// mediaEvent 是一条图片消息事件，图片地址由测试替换。
const mediaEvent = `{
  "version": "1.0",
  "header": {"eventId": "evt-media", "eventTime": 1761936492000, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明"},
    "chat": {"chatId": "20001", "chatType": "group"},
    "message": {
      "msgId": "5f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "chatId": "20001",
      "chatType": "group",
      "contentType": "image",
      "content": {"imageUrl": "SOURCE", "imageName": "a.png"}
    }
  }
}`

// imageData 取出首段图片数据。
func imageData(t *testing.T, event *obmodel.MessageEvent) obmodel.ImageData {
	t.Helper()

	if len(event.Message) == 0 || event.Message[0].Type != "image" {
		t.Fatalf("消息段为 %+v", event.Message)
	}
	data, ok := event.Message[0].Data.(obmodel.ImageData)
	if !ok {
		t.Fatalf("消息段数据为 %T", event.Message[0].Data)
	}
	return data
}

func TestImageUsesProxyLink(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer upstream.Close()

	payload := strings.ReplaceAll(mediaEvent, "SOURCE", upstream.URL+"/a.png")
	converter := newConverter()
	converter.Media = &media.Loader{HTTPClient: upstream.Client()}
	converter.MediaBaseURL = "http://127.0.0.1:8082"

	got, _, err := converter.Convert(context.Background(), parse(t, payload))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	data := imageData(t, got.(*obmodel.MessageEvent))
	if !strings.HasPrefix(data.File, "base64://") {
		t.Errorf("内容应内联为 base64，当前为 %q", data.File)
	}

	want := "http://127.0.0.1:8082" + proxypath.Media +
		base64.RawURLEncoding.EncodeToString([]byte(upstream.URL+"/a.png"))
	if data.URL != want {
		t.Errorf("地址为 %q，期望 %q", data.URL, want)
	}
}

func TestImageWithoutProxyHasNoLink(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer upstream.Close()

	payload := strings.ReplaceAll(mediaEvent, "SOURCE", upstream.URL+"/a.png")
	converter := newConverter()
	converter.Media = &media.Loader{HTTPClient: upstream.Client()}

	got, _, err := converter.Convert(context.Background(), parse(t, payload))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	if data := imageData(t, got.(*obmodel.MessageEvent)); data.URL != "" {
		t.Errorf("代理未启用时不应给出地址，当前为 %q", data.URL)
	}
}

func TestImageFallbackUsesProxyLink(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer upstream.Close()

	payload := strings.ReplaceAll(mediaEvent, "SOURCE", upstream.URL+"/a.png")
	converter := newConverter()
	converter.Media = &media.Loader{HTTPClient: upstream.Client()}
	converter.MediaBaseURL = "http://127.0.0.1:8082"

	got, _, err := converter.Convert(context.Background(), parse(t, payload))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}

	// 下载失败时不能再把原始地址交给客户端，否则客户端仍然会得到 403。
	data := imageData(t, got.(*obmodel.MessageEvent))
	if data.File == upstream.URL+"/a.png" {
		t.Errorf("降级时不应给出原始地址: %q", data.File)
	}
	if !strings.HasPrefix(data.File, "http://127.0.0.1:8082/media/") {
		t.Errorf("降级时 file 应为代理地址，当前为 %q", data.File)
	}
	if !strings.HasPrefix(data.URL, "http://127.0.0.1:8082/media/") {
		t.Errorf("降级时地址应为代理地址，当前为 %q", data.URL)
	}
}

func TestLinkIgnoresEmptySource(t *testing.T) {
	converter := newConverter()
	converter.MediaBaseURL = "http://127.0.0.1:8082"

	if got := converter.link(""); got != "" {
		t.Errorf("空地址不应生成链接，当前为 %q", got)
	}
	if got := converter.address("https://example.com/a.png"); got == "" {
		t.Error("代理未启用时应返回原地址")
	}
}
