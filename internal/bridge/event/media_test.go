// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

func TestFileEventDownloadsInline(t *testing.T) {
	// 文件消息的 content 与真实事件一致，只是地址换成本地服务。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("file body"))
	}))
	defer server.Close()

	raw := fmt.Sprintf(`{"version":"1.0","header":{"eventId":"1","eventType":"message.receive.normal","eventTime":1},"event":{"sender":{"senderId":"10001","senderType":"user","senderUserLevel":"member","senderNickname":"甲"},"chat":{"chatId":"20001","chatType":"group"},"message":{"msgId":"abc","chatId":"20001","chatType":"group","contentType":"file","content":{"fileName":"a.txt","fileUrl":%q,"fileSize":9}}}}`, server.URL+"/a.txt")

	var envelope model.Envelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("解析事件失败: %v", err)
	}

	converter := &Converter{SelfID: 1, Messages: identity.NewMemory(), Media: &media.Loader{}}
	converted, ok, err := converter.Convert(context.Background(), envelope)
	if err != nil || !ok {
		t.Fatalf("转换失败: %v，ok=%v", err, ok)
	}

	message := converted.(*obmodel.MessageEvent)
	if len(message.Message) != 1 || message.Message[0].Type != "file" {
		t.Fatalf("消息段为 %#v", message.Message)
	}

	data, ok := message.Message[0].Data.(obmodel.FileData)
	if !ok {
		t.Fatalf("文件段为 %T", message.Message[0].Data)
	}
	want := "base64://" + base64.StdEncoding.EncodeToString([]byte("file body"))
	if data.File != want {
		t.Errorf("文件内容为 %q", data.File)
	}
	if data.Name != "a.txt" {
		t.Errorf("文件名为 %q", data.Name)
	}
}
