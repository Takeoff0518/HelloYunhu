// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
)

// recordedCall 记录一次群管理类请求。
type recordedCall struct {
	path string
	body map[string]any
}

// newManageDispatcher 构造一个只处理撤回、禁言与踢人的测试实例。
func newManageDispatcher(t *testing.T) (*Dispatcher, *[]recordedCall) {
	t.Helper()

	var calls []recordedCall
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("解析请求失败: %v", err)
			}
		}
		calls = append(calls, recordedCall{path: r.URL.Path, body: body})
		_, _ = io.WriteString(w, `{"code":1,"msg":"success"}`)
	}))
	t.Cleanup(server.Close)

	client, err := api.New(api.Config{BaseURL: server.URL + "/open-apis/v1", Token: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	dispatcher := &Dispatcher{
		SelfID:   1001,
		API:      client,
		Messages: identity.NewMemory(),
	}

	return dispatcher, &calls
}

func TestDeleteMessage(t *testing.T) {
	dispatcher, calls := newManageDispatcher(t)

	messageID, err := dispatcher.Messages.MessageID(context.Background(), "5a89c003122442e38fef935e03df5fd3")
	if err != nil {
		t.Fatalf("分配标识失败: %v", err)
	}
	if err := dispatcher.Messages.SaveOrigin(context.Background(), messageID, "695367192", "group"); err != nil {
		t.Fatalf("记录归属失败: %v", err)
	}

	params := json.RawMessage(`{"message_id":1}`)
	if _, err := dispatcher.Handle(context.Background(), "delete_msg", params); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}

	if len(*calls) != 1 || (*calls)[0].path != "/open-apis/v1/bot/recall" {
		t.Fatalf("请求路径为 %#v", *calls)
	}
	body := (*calls)[0].body
	if body["msgId"] != "5a89c003122442e38fef935e03df5fd3" {
		t.Errorf("消息标识为 %v", body["msgId"])
	}
	if body["chatId"] != "695367192" || body["chatType"] != "group" {
		t.Errorf("会话信息为 %v", body)
	}
}

func TestDeleteMessageMapsPrivateChatType(t *testing.T) {
	dispatcher, calls := newManageDispatcher(t)

	messageID, err := dispatcher.Messages.MessageID(context.Background(), "abc")
	if err != nil {
		t.Fatalf("分配标识失败: %v", err)
	}
	// 私聊事件里的会话类型是 bot，撤回接口要的是 user。
	if err := dispatcher.Messages.SaveOrigin(context.Background(), messageID, "2272345", "bot"); err != nil {
		t.Fatalf("记录归属失败: %v", err)
	}

	if _, err := dispatcher.Handle(context.Background(), "delete_msg", json.RawMessage(`{"message_id":1}`)); err != nil {
		t.Fatalf("撤回失败: %v", err)
	}

	if body := (*calls)[0].body; body["chatType"] != "user" {
		t.Errorf("会话类型为 %v", body["chatType"])
	}
}

func TestDeleteMessageUnknownID(t *testing.T) {
	dispatcher, calls := newManageDispatcher(t)

	_, err := dispatcher.Handle(context.Background(), "delete_msg", json.RawMessage(`{"message_id":99}`))
	if err == nil {
		t.Fatal("未知消息应当返回错误")
	}

	var actionError *obmodel.ActionError
	if !errors.As(err, &actionError) {
		t.Fatalf("错误类型为 %T", err)
	}
	if actionError.Retcode != obmodel.RetcodeNotFound {
		t.Errorf("错误码为 %d", actionError.Retcode)
	}
	if len(*calls) != 0 {
		t.Errorf("不应发出请求: %#v", *calls)
	}
}

func TestSetGroupBan(t *testing.T) {
	dispatcher, calls := newManageDispatcher(t)

	// 60 秒不在平台的档位里，换算为 600 秒。
	params := json.RawMessage(`{"group_id":695367192,"user_id":2272345,"duration":60}`)
	if _, err := dispatcher.Handle(context.Background(), "set_group_ban", params); err != nil {
		t.Fatalf("禁言失败: %v", err)
	}

	if len(*calls) != 1 || (*calls)[0].path != "/open-apis/v1/group/gag-member" {
		t.Fatalf("请求路径为 %#v", *calls)
	}
	body := (*calls)[0].body
	if body["groupId"] != "695367192" || body["userId"] != "2272345" {
		t.Errorf("请求内容为 %v", body)
	}
	if gag, ok := body["gag"].(float64); !ok || int(gag) != 600 {
		t.Errorf("禁言时长为 %v", body["gag"])
	}
}

func TestNormalizeBanDuration(t *testing.T) {
	tests := []struct {
		seconds int
		want    int
		exact   bool
	}{
		{seconds: 0, want: 0, exact: true},
		{seconds: -1, want: -1, exact: true},
		{seconds: -100, want: -1, exact: true},
		{seconds: 1, want: 600, exact: false},
		{seconds: 60, want: 600, exact: false},
		{seconds: 600, want: 600, exact: true},
		{seconds: 601, want: 3600, exact: false},
		{seconds: 3600, want: 3600, exact: true},
		{seconds: 21600, want: 21600, exact: true},
		{seconds: 43200, want: 43200, exact: true},
		{seconds: 86400, want: 43200, exact: false},
	}

	for _, tt := range tests {
		got, exact := normalizeBanDuration(tt.seconds)
		if got != tt.want || exact != tt.exact {
			t.Errorf("%d 秒换算为 %d（exact=%v），期望 %d（exact=%v）",
				tt.seconds, got, exact, tt.want, tt.exact)
		}
	}
}

func TestSetGroupKick(t *testing.T) {
	dispatcher, calls := newManageDispatcher(t)

	params := json.RawMessage(`{"group_id":695367192,"user_id":2272345,"reject_add_request":true}`)
	if _, err := dispatcher.Handle(context.Background(), "set_group_kick", params); err != nil {
		t.Fatalf("踢人失败: %v", err)
	}

	if len(*calls) != 1 || (*calls)[0].path != "/open-apis/v1/group/remove-member" {
		t.Fatalf("请求路径为 %#v", *calls)
	}
	body := (*calls)[0].body
	if body["groupId"] != "695367192" || body["userId"] != "2272345" {
		t.Errorf("请求内容为 %v", body)
	}
}

func TestManageActionsBadParams(t *testing.T) {
	dispatcher, _ := newManageDispatcher(t)

	tests := []struct {
		name   string
		action string
		params string
	}{
		{name: "禁言缺少群标识", action: "set_group_ban", params: `{"user_id":1,"duration":60}`},
		{name: "禁言缺少成员标识", action: "set_group_ban", params: `{"group_id":1,"duration":60}`},
		{name: "踢人缺少群标识", action: "set_group_kick", params: `{"user_id":1}`},
		{name: "撤回缺少消息标识", action: "delete_msg", params: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := dispatcher.Handle(context.Background(), tt.action, json.RawMessage(tt.params)); err == nil {
				t.Fatal("应当返回错误")
			}
		})
	}
}
