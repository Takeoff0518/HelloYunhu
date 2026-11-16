// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
)

// tagServer 模拟群标签相关的接口。
type tagServer struct {
	mu             sync.Mutex
	tags           []string
	failCreate     bool
	failAdd        bool
	failRemoveTags map[string]bool
	tagPermission  bool
	calls          []string
}

// callLog 返回按顺序记录的调用。查询标签列表也算一次，便于确认调用次数。
func (s *tagServer) callLog() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.calls...)
}

// mutations 只返回改动类的调用，去掉查询。
func (s *tagServer) mutations() []string {
	calls := s.callLog()
	result := make([]string, 0, len(calls))
	for _, call := range calls {
		if call == "list" {
			continue
		}
		result = append(result, call)
	}
	return result
}

// handle 处理模拟请求。
func (s *tagServer) handle(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		var body map[string]json.RawMessage
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("解析请求体失败: %v", err)
			}
		}
		tag := ""
		if value, ok := body["tag"]; ok {
			_ = json.Unmarshal(value, &tag)
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		switch r.URL.Path {
		case "/open-apis/v1/group/tag/list":
			s.calls = append(s.calls, "list")
			items := make([]map[string]any, 0, len(s.tags))
			for index, name := range s.tags {
				items = append(items, map[string]any{
					"tag": name, "desc": "", "color": "#FF5733", "sort": index,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 1,
				"data": map[string]any{"list": items},
			})

		case "/open-apis/v1/group/tag/create":
			s.calls = append(s.calls, "create:"+tag)
			if s.failCreate {
				_, _ = io.WriteString(w, `{"code":-1,"msg":"机器人无权限"}`)
				return
			}
			s.tags = append(s.tags, tag)
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)

		case "/open-apis/v1/group/tag/user-relate":
			s.calls = append(s.calls, "add:"+tag)
			if s.failAdd {
				_, _ = io.WriteString(w, `{"code":-1,"msg":"标签不存在"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)

		case "/open-apis/v1/group/tag/user-relate-cancel":
			s.calls = append(s.calls, "remove:"+tag)
			if s.failRemoveTags[tag] {
				_, _ = io.WriteString(w, `{"code":-1,"msg":"用户未关联该标签"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)

		case "/open-apis/v1/group/tag/edit":
			s.calls = append(s.calls, "edit:"+tag)
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)

		case "/open-apis/v1/group/tag/delete":
			s.calls = append(s.calls, "delete:"+tag)
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)

		case "/open-apis/v1/group/info":
			allow := "0"
			if s.tagPermission {
				allow = "1"
			}
			_, _ = io.WriteString(w, `{"code":1,"data":{"groupId":"20001",
				"permissions":{"allowGroupTagManage":`+allow+`}}}`)

		default:
			t.Errorf("请求路径为 %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// newTagDispatcher 构造一个按标签接口回答的测试实例。
func newTagDispatcher(t *testing.T, options TagOptions) (*Dispatcher, *tagServer) {
	t.Helper()

	state := &tagServer{
		tags:           []string{"VIP用户", "活跃用户", "元老"},
		failRemoveTags: map[string]bool{},
	}
	server := httptest.NewServer(state.handle(t))
	t.Cleanup(server.Close)

	client, err := api.New(api.Config{
		BaseURL:    server.URL + "/open-apis/v1",
		Token:      "token",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	dispatcher := &Dispatcher{
		SelfID:   testSelfID,
		Nickname: "机器人",
		API:      client,
		Messages: identity.NewMemory(),
		Tags:     options,
	}

	return dispatcher, state
}

// replaceOptions 是默认的替换语义配置。
func replaceOptions() TagOptions {
	return TagOptions{AutoCreate: true, Color: "#03A9F4", Replace: true, ClearOnEmpty: true}
}

// handleTitle 调用 set_group_special_title。
func handleTitle(t *testing.T, dispatcher *Dispatcher, params string) error {
	t.Helper()

	_, err := dispatcher.Handle(context.Background(), "set_group_special_title", json.RawMessage(params))
	return err
}

func TestGetGroupTagsAction(t *testing.T) {
	dispatcher, _ := newTagDispatcher(t, replaceOptions())

	value, err := dispatcher.Handle(context.Background(), "yunhu_get_group_tags", json.RawMessage(`{"group_id":20001}`))
	if err != nil {
		t.Fatalf("查询群标签失败: %v", err)
	}

	list, ok := value.([]map[string]any)
	if !ok {
		t.Fatalf("响应类型为 %T", value)
	}
	if len(list) != 3 {
		t.Fatalf("标签数量为 %d", len(list))
	}
	if list[0]["tag"] != "VIP用户" {
		t.Errorf("标签为 %v", list[0])
	}
}

func TestTagActionsRejectBadInput(t *testing.T) {
	tests := []struct {
		name   string
		action string
		params string
	}{
		{name: "缺少群标识", action: "yunhu_create_group_tag", params: `{"tag":"VIP"}`},
		{name: "缺少标签名", action: "yunhu_create_group_tag", params: `{"group_id":20001}`},
		{name: "标签名过长", action: "yunhu_create_group_tag", params: `{"group_id":20001,"tag":"一二三四五六七八九十"}`},
		{name: "颜色格式错误", action: "yunhu_create_group_tag", params: `{"group_id":20001,"tag":"VIP","color":"红色"}`},
		{name: "修改时没有给出字段", action: "yunhu_edit_group_tag", params: `{"group_id":20001,"tag":"VIP"}`},
		{name: "加标签缺少成员", action: "yunhu_add_group_member_tag", params: `{"group_id":20001,"tag":"VIP"}`},
		{name: "移除标签缺少成员", action: "yunhu_remove_group_member_tag", params: `{"group_id":20001,"tag":"VIP"}`},
		{name: "头衔过长", action: "set_group_special_title", params: `{"group_id":20001,"user_id":2272345,"special_title":"一二三四五六七八九十"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher, state := newTagDispatcher(t, replaceOptions())

			if _, err := dispatcher.Handle(context.Background(), tt.action, json.RawMessage(tt.params)); err == nil {
				t.Fatal("应当返回错误")
			}
			if len(state.callLog()) != 0 {
				t.Errorf("参数不合法时不应发起请求: %v", state.callLog())
			}
		})
	}
}

func TestSetSpecialTitleReplace(t *testing.T) {
	dispatcher, state := newTagDispatcher(t, replaceOptions())

	if err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":"活跃用户"}`); err != nil {
		t.Fatalf("设置头衔失败: %v", err)
	}

	// 先加目标标签，再解绑其余标签。
	want := []string{"add:活跃用户", "remove:VIP用户", "remove:元老"}
	if got := state.mutations(); !reflect.DeepEqual(got, want) {
		t.Errorf("调用顺序为 %v，期望 %v", got, want)
	}
}

func TestSetSpecialTitleAppend(t *testing.T) {
	options := replaceOptions()
	options.Replace = false

	dispatcher, state := newTagDispatcher(t, options)

	// duration 对标签没有意义，传给平台之外不应报错。
	params := `{"group_id":20001,"user_id":2272345,"special_title":"活跃用户","duration":600}`
	if err := handleTitle(t, dispatcher, params); err != nil {
		t.Fatalf("设置头衔失败: %v", err)
	}

	want := []string{"add:活跃用户"}
	if got := state.mutations(); !reflect.DeepEqual(got, want) {
		t.Errorf("调用为 %v，期望 %v", got, want)
	}
}

func TestSetSpecialTitleClearsAll(t *testing.T) {
	dispatcher, state := newTagDispatcher(t, replaceOptions())

	if err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":""}`); err != nil {
		t.Fatalf("清空头衔失败: %v", err)
	}

	want := []string{"remove:VIP用户", "remove:活跃用户", "remove:元老"}
	if got := state.mutations(); !reflect.DeepEqual(got, want) {
		t.Errorf("调用为 %v，期望 %v", got, want)
	}
}

func TestSetSpecialTitleClearDisabled(t *testing.T) {
	options := replaceOptions()
	options.ClearOnEmpty = false

	dispatcher, state := newTagDispatcher(t, options)

	err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":""}`)
	if err == nil {
		t.Fatal("关闭清空后应当返回错误")
	}
	if !strings.Contains(err.Error(), "clear_on_empty") {
		t.Errorf("错误为 %q，应当指出对应的配置项", err)
	}
	if len(state.mutations()) != 0 {
		t.Errorf("不应发起改动: %v", state.mutations())
	}
}

func TestSetSpecialTitleAutoCreate(t *testing.T) {
	options := replaceOptions()
	options.Color = "#123456"

	dispatcher, state := newTagDispatcher(t, options)
	state.tags = nil

	if err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":"新人"}`); err != nil {
		t.Fatalf("设置头衔失败: %v", err)
	}

	want := []string{"create:新人", "add:新人"}
	if got := state.mutations(); !reflect.DeepEqual(got, want) {
		t.Errorf("调用为 %v，期望 %v", got, want)
	}
}

func TestSetSpecialTitleMissingTag(t *testing.T) {
	options := replaceOptions()
	options.AutoCreate = false

	dispatcher, state := newTagDispatcher(t, options)

	err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":"新人"}`)
	if err == nil {
		t.Fatal("标签不存在时应当返回错误")
	}

	var actionError *obmodel.ActionError
	if !errors.As(err, &actionError) {
		t.Fatalf("错误类型为 %T", err)
	}
	if !strings.Contains(actionError.Wording, "VIP用户、活跃用户、元老") {
		t.Errorf("说明为 %q，应当列出已有标签", actionError.Wording)
	}
	if len(state.mutations()) != 0 {
		t.Errorf("不应发起改动: %v", state.mutations())
	}
}

func TestUnbindToleratesSingleFailure(t *testing.T) {
	dispatcher, state := newTagDispatcher(t, replaceOptions())
	// 一个标签解绑失败不应影响其余标签的处理。
	state.failRemoveTags["VIP用户"] = true

	if err := handleTitle(t, dispatcher, `{"group_id":20001,"user_id":2272345,"special_title":"活跃用户"}`); err != nil {
		t.Fatalf("部分失败时仍应成功: %v", err)
	}

	want := []string{"add:活跃用户", "remove:VIP用户", "remove:元老"}
	if got := state.mutations(); !reflect.DeepEqual(got, want) {
		t.Errorf("调用为 %v，期望 %v", got, want)
	}
}

func TestTagFailureExplainsPermission(t *testing.T) {
	dispatcher, state := newTagDispatcher(t, replaceOptions())
	state.failCreate = true
	state.tagPermission = false
	state.tags = nil

	params := `{"group_id":20001,"tag":"新人"}`
	_, err := dispatcher.Handle(context.Background(), "yunhu_create_group_tag", json.RawMessage(params))
	if err == nil {
		t.Fatal("创建标签应当失败")
	}

	var actionError *obmodel.ActionError
	if !errors.As(err, &actionError) {
		t.Fatalf("错误类型为 %T", err)
	}
	if !strings.Contains(actionError.Wording, "allowGroupTagManage") {
		t.Errorf("说明为 %q，应当指出缺少的权限", actionError.Wording)
	}
}
