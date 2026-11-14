// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// sentRequest 记录测试服务收到的发送请求。
type sentRequest struct {
	mu       sync.Mutex
	requests []yunhumodel.SendMessageRequest
}

// newDispatcher 启动模拟的云湖接口服务并返回处理器。
func newDispatcher(t *testing.T) (*Dispatcher, *sentRequest) {
	t.Helper()

	recorder := &sentRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/bot/send" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}

		var request yunhumodel.SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("解析请求失败: %v", err)
		}
		recorder.mu.Lock()
		recorder.requests = append(recorder.requests, request)
		recorder.mu.Unlock()

		_, _ = io.WriteString(w, `{"code":0,"data":{"msgId":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}`)
	}))
	t.Cleanup(server.Close)

	client, err := api.New(api.Config{BaseURL: server.URL + "/open-apis/v1", Token: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	return &Dispatcher{
		SelfID:   1001,
		Nickname: "机器人",
		API:      client,
		Messages: identity.NewMemory(),
	}, recorder
}

// last 返回最后一次发送请求。
func (r *sentRequest) last(t *testing.T) yunhumodel.SendMessageRequest {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.requests) == 0 {
		t.Fatal("没有收到发送请求")
	}
	return r.requests[len(r.requests)-1]
}

// textParams 构造发送参数。
func textParams(t *testing.T, extra map[string]any, text string) json.RawMessage {
	t.Helper()

	params := map[string]any{
		"message": []map[string]any{
			{"type": "text", "data": map[string]any{"text": text}},
		},
	}
	for key, value := range extra {
		params[key] = value
	}

	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}
	return raw
}

func TestSendGroupMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	data, err := dispatcher.Handle(context.Background(), "send_group_msg", textParams(t, map[string]any{"group_id": 20001}, "你好"))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	if request.RecvType != yunhumodel.RecvTypeGroup || request.RecvID != "20001" {
		t.Errorf("接收方为 %s/%s", request.RecvType, request.RecvID)
	}
	if request.ContentType != yunhumodel.ContentTypeText {
		t.Errorf("内容类型为 %q", request.ContentType)
	}

	content := request.Content.(map[string]any)
	if content["text"] != "你好" {
		t.Errorf("正文为 %v", content["text"])
	}

	payload := data.(map[string]any)
	if payload["message_id"] != int64(1) {
		t.Errorf("消息标识为 %v", payload["message_id"])
	}
}

func TestSendPrivateMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	if _, err := dispatcher.Handle(context.Background(), "send_private_msg", textParams(t, map[string]any{"user_id": 10001}, "在吗")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	if request.RecvType != yunhumodel.RecvTypeUser || request.RecvID != "10001" {
		t.Errorf("接收方为 %s/%s", request.RecvType, request.RecvID)
	}
}

func TestSendMessageRoutesByType(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	if _, err := dispatcher.Handle(context.Background(), "send_msg", textParams(t, map[string]any{"message_type": "group", "group_id": 20001}, "群聊")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if got := recorder.last(t).RecvType; got != yunhumodel.RecvTypeGroup {
		t.Errorf("接收方类型为 %q", got)
	}

	if _, err := dispatcher.Handle(context.Background(), "send_msg", textParams(t, map[string]any{"message_type": "private", "user_id": 10001}, "私聊")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if got := recorder.last(t).RecvType; got != yunhumodel.RecvTypeUser {
		t.Errorf("接收方类型为 %q", got)
	}
}

func TestSendIncludesAt(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"at","data":{"qq":10002}},{"type":"text","data":{"text":" 你好"}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	content := request.Content.(map[string]any)
	at, ok := content["at"].([]any)
	if !ok || len(at) != 1 || at[0] != "10002" {
		t.Errorf("at 列表为 %#v", content["at"])
	}
}

func TestSendReplyUsesParentID(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	// 先注册一条收到的消息，模拟框架引用它。
	messageID, err := dispatcher.Messages.MessageID(context.Background(), "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("注册消息标识失败: %v", err)
	}

	params := map[string]any{
		"group_id": 20001,
		"message": []map[string]any{
			{"type": "reply", "data": map[string]any{"id": messageID}},
			{"type": "text", "data": map[string]any{"text": "回复"}},
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}

	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", raw); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	if got := recorder.last(t).ParentID; got != "0f1e2d3c4b5a69788796a5b4c3d2e1f0" {
		t.Errorf("引用目标为 %q", got)
	}
}

func TestSendReplyWithUnknownIDIsIgnored(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"reply","data":{"id":999}},{"type":"text","data":{"text":"回复"}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	if got := recorder.last(t).ParentID; got != "" {
		t.Errorf("引用目标应为空，实际为 %q", got)
	}
}

func TestSendBadParams(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	tests := []struct {
		name   string
		action string
		params string
	}{
		{name: "缺少群标识", action: "send_group_msg", params: `{"message":[]}`},
		{name: "缺少用户标识", action: "send_private_msg", params: `{"message":[]}`},
		{name: "消息类型不支持", action: "send_msg", params: `{"message_type":"discuss","message":[]}`},
		{name: "参数无法解析", action: "send_group_msg", params: `{"group_id":"abc"}`},
		{name: "空消息", action: "send_group_msg", params: `{"group_id":20001,"message":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dispatcher.Handle(context.Background(), tt.action, json.RawMessage(tt.params))

			var actionErr *obmodel.ActionError
			if !errors.As(err, &actionErr) {
				t.Fatalf("错误类型为 %T: %v", err, err)
			}
			if actionErr.Retcode != obmodel.RetcodeBadRequest {
				t.Errorf("返回码为 %d，期望 %d", actionErr.Retcode, obmodel.RetcodeBadRequest)
			}
		})
	}
}

func TestUnimplementedAction(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	_, err := dispatcher.Handle(context.Background(), "get_group_list", json.RawMessage(`{}`))

	var actionErr *obmodel.ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("错误类型为 %T", err)
	}
	if actionErr.Retcode != obmodel.RetcodeNotFound {
		t.Errorf("返回码为 %d，期望 1404", actionErr.Retcode)
	}
	if !strings.Contains(actionErr.Wording, "get_group_list") {
		t.Errorf("说明为 %q", actionErr.Wording)
	}
}

func TestMetaActions(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	info, err := dispatcher.Handle(context.Background(), "get_login_info", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	login := info.(map[string]any)
	if login["user_id"] != int64(1001) || login["nickname"] != "机器人" {
		t.Errorf("登录信息为 %v", login)
	}

	status, err := dispatcher.Handle(context.Background(), "get_status", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if status.(map[string]any)["online"] != true {
		t.Errorf("状态为 %v", status)
	}

	version, err := dispatcher.Handle(context.Background(), "get_version_info", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if version.(map[string]any)["protocol_version"] != "v11" {
		t.Errorf("版本信息为 %v", version)
	}

	if _, err := dispatcher.Handle(context.Background(), ".handle_quick_operation_async", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("快速操作应当返回成功: %v", err)
	}
}
