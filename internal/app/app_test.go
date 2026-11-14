// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{}

// writeConfig 把配置写入临时目录并返回路径。
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

// wsURL 把 HTTP 服务地址改写为 WebSocket 地址。
func wsURL(t *testing.T, server *httptest.Server, path string) string {
	t.Helper()
	return "ws" + strings.TrimPrefix(server.URL, "http") + path
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

func TestRunInstanceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":401,"msg":"令牌无效"}`))
	}))
	defer server.Close()

	config := `
yunhu:
  api_base: ` + server.URL + `/open-apis/v1
  subscribe_url: ws://127.0.0.1:1/subscribe
instances:
  - name: main
    token: bad-token
    onebot:
      url: ws://127.0.0.1:1/ws
`
	var logs bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Run(ctx, Options{ConfigPath: writeConfig(t, config), LogOutput: &logs})
	if err == nil {
		t.Fatal("实例启动失败时应返回错误")
	}
	for _, want := range []string{"实例 main", "获取机器人信息", "令牌无效"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息中缺少 %q，实际为: %v", want, err)
		}
	}
	if strings.Contains(logs.String(), "bad-token") {
		t.Error("日志不应输出机器人令牌")
	}
}

// groupMessageEvent 是模拟订阅连接推送的群聊消息。
const groupMessageEvent = `{
  "version": "1.0",
  "header": {"eventId": "evt-1", "eventTime": 1761936492340, "eventType": "message.receive.normal"},
  "event": {
    "sender": {"senderId": "10001", "senderNickname": "小明"},
    "chat": {"chatId": "20001", "chatType": "group"},
    "message": {
      "msgId": "0f1e2d3c4b5a69788796a5b4c3d2e1f0",
      "chatId": "20001",
      "chatType": "group",
      "contentType": "text",
      "content": {"text": "@机器人 你好", "at": ["1001"]}
    }
  }
}`

func TestRunEndToEnd(t *testing.T) {
	sendRequests := make(chan map[string]any, 1)

	// 模拟云湖：机器人信息、发送消息与订阅连接。
	yunhuServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/v1/bot/info":
			_, _ = w.Write([]byte(`{"code":0,"data":{"botId":"1001","nickname":"机器人"}}`))

		case "/open-apis/v1/bot/send":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("解析发送请求失败: %v", err)
			}
			sendRequests <- request
			_, _ = w.Write([]byte(`{"code":0,"data":{"msgId":"1f1e2d3c4b5a69788796a5b4c3d2e1f0"}}`))

		case "/subscribe":
			if got := r.URL.Query().Get("token"); got != "test-token" {
				t.Errorf("订阅令牌为 %q", got)
			}
			conn, err := testUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer func() {
				_ = conn.Close()
			}()

			if err := conn.WriteMessage(websocket.TextMessage, []byte(groupMessageEvent)); err != nil {
				return
			}
			<-r.Context().Done()

		default:
			http.NotFound(w, r)
		}
	}))
	defer yunhuServer.Close()

	// 模拟机器人框架：反向 WebSocket 服务端。
	events := make(chan map[string]any, 4)
	responses := make(chan map[string]any, 1)
	headers := make(chan http.Header, 1)

	frameworkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Clone()

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var frame map[string]any
			if err := json.Unmarshal(payload, &frame); err != nil {
				t.Errorf("解析框架收到的帧失败: %v", err)
				return
			}

			if _, isEvent := frame["post_type"]; isEvent {
				events <- frame

				// 收到消息事件后立刻发一次发送调用，模拟框架回复。
				if frame["post_type"] == "message" {
					action := `{"action":"send_group_msg","params":{"group_id":20001,"message":[{"type":"text","data":{"text":"回复"}}],"self_id":1001},"echo":{"seq":1}}`
					if err := conn.WriteMessage(websocket.TextMessage, []byte(action)); err != nil {
						return
					}
				}
				continue
			}

			responses <- frame
		}
	}))
	defer frameworkServer.Close()

	config := `
log:
  level: debug
  format: text
yunhu:
  api_base: ` + yunhuServer.URL + `/open-apis/v1
  subscribe_url: ` + wsURL(t, yunhuServer, "/subscribe") + `
instances:
  - name: main
    token: test-token
    onebot:
      url: ` + wsURL(t, frameworkServer, "/ws") + `
`

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- Run(ctx, Options{ConfigPath: writeConfig(t, config), Version: "test", LogOutput: &logs})
	}()

	// 握手应当带上正确的头部。
	select {
	case header := <-headers:
		if got := header.Get("X-Self-ID"); got != "1001" {
			t.Errorf("X-Self-ID 为 %q", got)
		}
		if got := header.Get("X-Client-Role"); got != "Universal" {
			t.Errorf("X-Client-Role 为 %q", got)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("框架没有收到握手")
	}

	// 第一条事件应当是连接建立。
	select {
	case event := <-events:
		if event["post_type"] != "meta_event" || event["meta_event_type"] != "lifecycle" {
			t.Errorf("第一条事件为 %v", event)
		}
		if event["sub_type"] != "connect" {
			t.Errorf("lifecycle 的 sub_type 为 %v", event["sub_type"])
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("框架没有收到生命周期事件")
	}

	// 第二条事件应当是转换后的群消息。
	select {
	case event := <-events:
		if event["post_type"] != "message" || event["message_type"] != "group" {
			t.Errorf("消息事件为 %v", event)
		}
		if event["group_id"] != float64(20001) || event["user_id"] != float64(10001) {
			t.Errorf("标识为 %v", event)
		}
		messageID, ok := event["message_id"].(float64)
		if !ok || messageID <= 0 {
			t.Errorf("message_id 为 %v，必须是可转换为整数的值", event["message_id"])
		}
		sender, ok := event["sender"].(map[string]any)
		if !ok || sender["nickname"] != "小明" {
			t.Errorf("sender 为 %v", event["sender"])
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("框架没有收到消息事件")
	}

	// 发送调用的响应应当带回原样 echo 与消息标识。
	select {
	case response := <-responses:
		if response["status"] != "ok" {
			t.Errorf("响应状态为 %v", response["status"])
		}
		echo, ok := response["echo"].(map[string]any)
		if !ok || echo["seq"] != float64(1) {
			t.Errorf("echo 为 %v", response["echo"])
		}
		data, ok := response["data"].(map[string]any)
		if !ok {
			t.Fatalf("data 为 %v", response["data"])
		}
		if _, ok := data["message_id"].(float64); !ok {
			t.Errorf("data.message_id 为 %v", data["message_id"])
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("框架没有收到发送响应")
	}

	// 云湖侧应当收到一条文本消息。
	select {
	case request := <-sendRequests:
		if request["recvId"] != "20001" || request["recvType"] != "group" {
			t.Errorf("接收方为 %v", request)
		}
		content, ok := request["content"].(map[string]any)
		if !ok || content["text"] != "回复" {
			t.Errorf("内容为 %v", request["content"])
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("云湖没有收到发送请求")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run 没有在取消后返回")
	}
}
