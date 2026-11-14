// SPDX-License-Identifier: GPL-3.0-or-later

package subscribe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

var testUpgrader = websocket.Upgrader{}

const sampleEnvelope = `{
  "version": "1.0",
  "header": {"eventId": "evt-1", "eventTime": 1761936492340, "eventType": "message.receive.normal"},
  "event": {"chat": {"chatId": "20001", "chatType": "group"}}
}`

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "协议不支持", cfg: Config{URL: "http://example.com/subscribe", Token: "t"}, wantErr: "协议必须是 ws 或 wss"},
		{name: "缺少主机名", cfg: Config{URL: "wss://", Token: "t"}, wantErr: "缺少主机名"},
		{name: "令牌为空", cfg: Config{URL: "wss://ws.example.com/subscribe"}, wantErr: "令牌不能为空"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Options{Config: tt.cfg})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("错误为 %v，期望包含 %q", err, tt.wantErr)
			}
		})
	}
}

// start 在后台运行客户端，返回事件通道。
func start(t *testing.T, client *Client) <-chan model.Envelope {
	t.Helper()

	events := make(chan model.Envelope, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		if err := client.Run(ctx, events); err != nil {
			t.Errorf("Run 返回错误: %v", err)
		}
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("订阅客户端没有在超时内停止")
		}
	})

	return events
}

func TestReceivesEnvelope(t *testing.T) {
	tokens := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokens <- r.URL.Query().Get("token")

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		if err := conn.WriteMessage(websocket.TextMessage, []byte(sampleEnvelope)); err != nil {
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Options{Config: Config{
		URL:   "ws" + strings.TrimPrefix(server.URL, "http") + "/subscribe",
		Token: "test-token",
	}})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	events := start(t, client)

	select {
	case token := <-tokens:
		if token != "test-token" {
			t.Errorf("令牌参数为 %q", token)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到订阅请求")
	}

	select {
	case envelope := <-events:
		if envelope.Header.EventType != model.EventTypeMessageNormal {
			t.Errorf("事件类型为 %q", envelope.Header.EventType)
		}
		if envelope.Header.EventID != "evt-1" {
			t.Errorf("事件标识为 %q", envelope.Header.EventID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到事件")
	}
}

func TestIgnoresBrokenFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		// 先发一帧坏数据，再发一帧正常事件，客户端应当跳过坏数据继续工作。
		if err := conn.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(sampleEnvelope)); err != nil {
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Options{Config: Config{
		URL:   "ws" + strings.TrimPrefix(server.URL, "http") + "/subscribe",
		Token: "test-token",
	}})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	events := start(t, client)

	select {
	case envelope := <-events:
		if envelope.Header.EventID != "evt-1" {
			t.Errorf("事件标识为 %q", envelope.Header.EventID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("坏帧之后的正常事件没有被处理")
	}
}

func TestReconnect(t *testing.T) {
	var handshakes atomic.Int32
	reconnected := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := handshakes.Add(1)

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		if count == 1 {
			return
		}

		reconnected <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Options{Config: Config{
		URL:   "ws" + strings.TrimPrefix(server.URL, "http") + "/subscribe",
		Token: "test-token",
	}})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	client.minDelay = 10 * time.Millisecond
	client.maxDelay = 50 * time.Millisecond

	start(t, client)

	select {
	case <-reconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("订阅客户端没有重连")
	}
}
