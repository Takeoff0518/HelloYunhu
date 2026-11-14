// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

var testUpgrader = websocket.Upgrader{}

func TestNewValidation(t *testing.T) {
	handler := func(context.Context, string, json.RawMessage) (any, error) { return nil, nil }

	tests := []struct {
		name    string
		cfg     Config
		handler Handler
		wantErr string
	}{
		{
			name:    "协议不支持",
			cfg:     Config{URL: "http://127.0.0.1:6199/ws", SelfID: 1},
			handler: handler,
			wantErr: "协议必须是 ws 或 wss",
		},
		{
			name:    "缺少主机名",
			cfg:     Config{URL: "ws://", SelfID: 1},
			handler: handler,
			wantErr: "缺少主机名",
		},
		{
			name:    "机器人标识不合法",
			cfg:     Config{URL: "ws://127.0.0.1:6199/ws", SelfID: 0},
			handler: handler,
			wantErr: "机器人标识必须是正整数",
		},
		{
			name:    "缺少处理器",
			cfg:     Config{URL: "ws://127.0.0.1:6199/ws", SelfID: 1},
			wantErr: "缺少接口处理器",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Options{Config: tt.cfg, Handler: tt.handler})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("错误为 %v，期望包含 %q", err, tt.wantErr)
			}
		})
	}
}

// startClient 在后台运行客户端，返回取消函数。
func startClient(t *testing.T, client *Client) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := client.Run(ctx); err != nil {
			t.Errorf("Run 返回错误: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("客户端没有在超时内停止")
		}
	})

	return cancel
}

func TestHandshakeAndResponse(t *testing.T) {
	responses := make(chan []byte, 1)
	handshake := make(chan http.Header, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handshake <- r.Header.Clone()

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		request := `{"action":"send_group_msg","params":{"group_id":1},"echo":{"seq":7}}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(request)); err != nil {
			return
		}

		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		responses <- payload
	}))
	defer server.Close()

	var gotAction string
	var gotParams json.RawMessage
	handler := func(_ context.Context, action string, params json.RawMessage) (any, error) {
		gotAction = action
		gotParams = params
		return map[string]any{"message_id": 42}, nil
	}

	client, err := New(Options{
		Config:  Config{URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", AccessToken: "secret", SelfID: 1001},
		Handler: handler,
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	startClient(t, client)

	select {
	case headers := <-handshake:
		if got := headers.Get("X-Self-ID"); got != "1001" {
			t.Errorf("X-Self-ID 为 %q", got)
		}
		if got := headers.Get("X-Client-Role"); got != "Universal" {
			t.Errorf("X-Client-Role 为 %q", got)
		}
		if got := headers.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization 为 %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到握手")
	}

	select {
	case payload := <-responses:
		var response model.Response
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if response.Status != model.StatusOK || response.Retcode != model.RetcodeOK {
			t.Errorf("响应为 %+v", response)
		}
		if string(response.Echo) != `{"seq":7}` {
			t.Errorf("回显为 %s，期望原样返回", response.Echo)
		}
		data, ok := response.Data.(map[string]any)
		if !ok || data["message_id"] != float64(42) {
			t.Errorf("数据为 %#v", response.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到响应")
	}

	if gotAction != "send_group_msg" {
		t.Errorf("action 为 %q", gotAction)
	}
	if string(gotParams) != `{"group_id":1}` {
		t.Errorf("params 为 %s", gotParams)
	}
}

func TestActionErrorBecomesFailedResponse(t *testing.T) {
	responses := make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		request := `{"action":"set_group_ban","params":{},"echo":{"seq":9}}`
		if err := conn.WriteMessage(websocket.TextMessage, []byte(request)); err != nil {
			return
		}

		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		responses <- payload
	}))
	defer server.Close()

	client, err := New(Options{
		Config: Config{URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", SelfID: 1001},
		Handler: func(context.Context, string, json.RawMessage) (any, error) {
			return nil, model.NotImplemented("set_group_ban")
		},
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	startClient(t, client)

	select {
	case payload := <-responses:
		var response model.Response
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if response.Status != model.StatusFailed {
			t.Errorf("状态为 %q，期望 failed", response.Status)
		}
		if response.Retcode != model.RetcodeNotFound {
			t.Errorf("返回码为 %d，期望 %d", response.Retcode, model.RetcodeNotFound)
		}
		if string(response.Echo) != `{"seq":9}` {
			t.Errorf("回显为 %s", response.Echo)
		}
		if !strings.Contains(response.Wording, "set_group_ban") {
			t.Errorf("说明为 %q", response.Wording)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到响应")
	}
}

func TestSendEvent(t *testing.T) {
	events := make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		events <- payload

		// 保持连接直到测试结束，避免客户端立即重连
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Options{
		Config:  Config{URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", SelfID: 1001},
		Handler: func(context.Context, string, json.RawMessage) (any, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	startClient(t, client)

	// 等到连接建立后再发送，连接建立前会返回 ErrNotConnected。
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := client.SendEvent(model.LifecycleEvent{
			PostType:      model.PostTypeMetaEvent,
			MetaEventType: model.MetaEventTypeLifecycle,
			SubType:       "connect",
			Time:          1761936492,
			SelfID:        1001,
		})
		if err == nil {
			break
		}
		if !errors.Is(err, ErrNotConnected) {
			t.Fatalf("发送事件失败: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("连接一直没有建立")
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case payload := <-events:
		var event model.LifecycleEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("解析事件失败: %v", err)
		}
		if event.PostType != model.PostTypeMetaEvent || event.MetaEventType != model.MetaEventTypeLifecycle {
			t.Errorf("事件为 %+v", event)
		}
		if event.SubType != "connect" {
			t.Errorf("子类型为 %q", event.SubType)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("没有收到事件")
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
			// 第一次连接立刻断开，验证客户端会重连。
			return
		}

		reconnected <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Options{
		Config:  Config{URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", SelfID: 1001},
		Handler: func(context.Context, string, json.RawMessage) (any, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	client.minDelay = 10 * time.Millisecond
	client.maxDelay = 50 * time.Millisecond
	startClient(t, client)

	select {
	case <-reconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("客户端没有重连")
	}
}

func TestSendEventWhenDisconnected(t *testing.T) {
	client, err := New(Options{
		Config:  Config{URL: "ws://127.0.0.1:1/ws", SelfID: 1001},
		Handler: func(context.Context, string, json.RawMessage) (any, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}

	if err := client.SendEvent(map[string]any{"post_type": "meta_event"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("错误为 %v，期望 ErrNotConnected", err)
	}
}
