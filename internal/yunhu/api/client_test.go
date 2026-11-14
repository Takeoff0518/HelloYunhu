// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "令牌为空",
			cfg:     Config{BaseURL: "https://example.com/open-apis/v1"},
			wantErr: "令牌不能为空",
		},
		{
			name:    "协议不支持",
			cfg:     Config{BaseURL: "ftp://example.com", Token: "t"},
			wantErr: "协议必须是 http 或 https",
		},
		{
			name:    "缺少主机名",
			cfg:     Config{BaseURL: "https://", Token: "t"},
			wantErr: "缺少主机名",
		},
		{
			name: "正常配置",
			cfg:  Config{BaseURL: "https://example.com/open-apis/v1/", Token: "t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("错误为 %v，期望包含 %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("New 返回错误: %v", err)
			}
		})
	}
}

// newTestClient 启动一个测试服务并把客户端指向它。
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := New(Config{BaseURL: server.URL + "/open-apis/v1", Token: "test-token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	return client
}

func TestBotInfo(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/open-apis/v1/bot/info" {
			t.Errorf("请求为 %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("token"); got != "test-token" {
			t.Errorf("令牌参数为 %q", got)
		}
		_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"botId":"1001","nickname":"测试机器人"}}`)
	})

	info, err := client.BotInfo(context.Background())
	if err != nil {
		t.Fatalf("BotInfo 返回错误: %v", err)
	}
	if info.BotID != "1001" || info.Nickname != "测试机器人" {
		t.Errorf("机器人信息为 %+v", info)
	}
}

func TestBotInfoMissingBotID(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":{"nickname":"没有标识"}}`)
	})

	_, err := client.BotInfo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "缺少 botId") {
		t.Fatalf("错误为 %v，期望说明缺少 botId", err)
	}
}

func TestBusinessError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":1007,"msg":"too many requests"}`)
	})

	_, err := client.BotInfo(context.Background())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型为 %T，期望 *APIError: %v", err, err)
	}
	if apiErr.Code != CodeRateLimited {
		t.Errorf("错误码为 %d，期望 %d", apiErr.Code, CodeRateLimited)
	}
	if !strings.Contains(apiErr.Error(), "too many requests") {
		t.Errorf("错误信息缺少平台说明: %v", apiErr)
	}
}

func TestUnexpectedStatus(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	})

	_, err := client.BotInfo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("错误为 %v，期望包含状态码 500", err)
	}
}

func TestResponseIsNotJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<html>网关错误</html>")
	})

	_, err := client.BotInfo(context.Background())
	if err == nil || !strings.Contains(err.Error(), "不是合法 JSON") {
		t.Fatalf("错误为 %v，期望说明响应格式错误", err)
	}
}

func TestSendMessage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/open-apis/v1/bot/send" {
			t.Errorf("请求为 %s %s", r.Method, r.URL.Path)
		}

		var req model.SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if req.RecvID != "123456" || req.RecvType != model.RecvTypeGroup {
			t.Errorf("接收方为 %s/%s", req.RecvID, req.RecvType)
		}
		if req.ContentType != model.ContentTypeText {
			t.Errorf("内容类型为 %q", req.ContentType)
		}
		content, ok := req.Content.(map[string]any)
		if !ok || content["text"] != "你好" {
			t.Errorf("内容为 %#v", req.Content)
		}

		_, _ = io.WriteString(w, `{"code":0,"data":{"msgId":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}`)
	})

	result, err := client.SendMessage(context.Background(), model.SendMessageRequest{
		RecvID:      "123456",
		RecvType:    model.RecvTypeGroup,
		ContentType: model.ContentTypeText,
		Content:     map[string]any{"text": "你好"},
	})
	if err != nil {
		t.Fatalf("SendMessage 返回错误: %v", err)
	}
	if result.ID() != "0f1e2d3c4b5a69788796a5b4c3d2e1f0" {
		t.Errorf("消息标识为 %q", result.ID())
	}
}

func TestSendMessageDataIsString(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}`)
	})

	result, err := client.SendMessage(context.Background(), model.SendMessageRequest{
		RecvID:      "123456",
		RecvType:    model.RecvTypeUser,
		ContentType: model.ContentTypeText,
		Content:     map[string]any{"text": "你好"},
	})
	if err != nil {
		t.Fatalf("SendMessage 返回错误: %v", err)
	}
	if result.ID() != "0f1e2d3c4b5a69788796a5b4c3d2e1f0" {
		t.Errorf("消息标识为 %q", result.ID())
	}
}

func TestSendMessageRejectsBadRequest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("请求不合法时不应发起网络调用")
	})

	tests := []struct {
		name string
		req  model.SendMessageRequest
	}{
		{name: "缺少接收方", req: model.SendMessageRequest{RecvType: model.RecvTypeGroup, ContentType: model.ContentTypeText}},
		{name: "接收方类型不支持", req: model.SendMessageRequest{RecvID: "1", RecvType: "channel", ContentType: model.ContentTypeText}},
		{name: "缺少内容类型", req: model.SendMessageRequest{RecvID: "1", RecvType: model.RecvTypeGroup}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := client.SendMessage(context.Background(), tt.req); err == nil {
				t.Fatal("期望返回错误")
			}
		})
	}
}

func TestEnvelopeAcceptsMsgField(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":1007,"msg":"请求过于频繁"}`)
	})

	_, err := client.BotInfo(context.Background())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("错误类型为 %T", err)
	}
	if !strings.Contains(apiErr.Error(), "请求过于频繁") {
		t.Errorf("错误信息缺少平台说明: %v", apiErr)
	}
}
