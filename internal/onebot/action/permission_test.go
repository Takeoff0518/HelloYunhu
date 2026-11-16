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
)

// 测试用的群标识与机器人标识。
const (
	testGroupID = "20001"
	testSelfID  = int64(1001)
)

// permissionServer 模拟群信息、群成员与禁言接口，可以按用例调整返回值。
type permissionServer struct {
	mu          sync.Mutex
	permissions string
	members     string
	infoFails   bool
	memberFails bool
	gagFails    bool
	memberCalls int
}

// setPermissions 替换群信息里的权限对象。
func (s *permissionServer) setPermissions(raw string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.permissions = raw
}

// memberCallCount 返回成员列表接口被调用的次数。
func (s *permissionServer) memberCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.memberCalls
}

// handle 处理模拟请求。
func (s *permissionServer) handle(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		switch r.URL.Path {
		case "/open-apis/v1/group/info":
			if s.infoFails {
				_, _ = io.WriteString(w, `{"code":1003,"msg":"机器人未加入该群"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{"groupId":"`+testGroupID+
				`","name":"测试群","headcount":3,"permissions":`+s.permissions+`}}`)

		case "/open-apis/v1/group/members":
			s.memberCalls++
			if s.memberFails {
				_, _ = io.WriteString(w, `{"code":1003,"msg":"机器人无获取群成员列表权限"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{"members":`+s.members+`}}`)

		case "/open-apis/v1/group/gag-member":
			if s.gagFails {
				_, _ = io.WriteString(w, `{"code":1003,"msg":"机器人无禁言权限"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":1,"msg":"success"}`)

		default:
			t.Errorf("请求路径为 %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// newPermissionDispatcher 构造一个按权限开关回答的测试实例。
func newPermissionDispatcher(t *testing.T) (*Dispatcher, *permissionServer) {
	t.Helper()

	state := &permissionServer{
		permissions: `{}`,
		members:     `[{"userId":"10001","nickname":"小明","role":"admin"}]`,
	}
	server := httptest.NewServer(state.handle(t))
	t.Cleanup(server.Close)

	client, err := api.New(api.Config{BaseURL: server.URL + "/open-apis/v1", Token: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	dispatcher := &Dispatcher{
		SelfID:   testSelfID,
		Nickname: "机器人",
		API:      client,
		Messages: identity.NewMemory(),
		// 默认按群主上报持有标签管理权限的机器人，与配置默认值一致。
		Tags: TagOptions{ReportOwner: true},
	}

	return dispatcher, state
}

// memberInfo 调用 get_group_member_info 并返回响应对象。
func memberInfo(t *testing.T, dispatcher *Dispatcher, params string) map[string]any {
	t.Helper()

	value, err := dispatcher.Handle(context.Background(), "get_group_member_info", json.RawMessage(params))
	if err != nil {
		t.Fatalf("查询成员信息失败: %v", err)
	}

	payload, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("响应类型为 %T", value)
	}
	return payload
}

func TestSelfMemberRoleFollowsPermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions string
		want        string
	}{
		{name: "未授权", permissions: `{}`, want: roleMember},
		{
			name:        "全部为 0",
			permissions: `{"allowEditGroupInfo":0,"allowGagMember":0,"allowRemoveMember":0,"allowGroupTagManage":0,"allowGetGroupMembers":0}`,
			want:        roleMember,
		},
		{name: "禁言", permissions: `{"allowGagMember":1}`, want: roleAdmin},
		{name: "移除成员", permissions: `{"allowRemoveMember":1}`, want: roleAdmin},
		{name: "修改群信息", permissions: `{"allowEditGroupInfo":1}`, want: roleAdmin},
		{name: "群标签", permissions: `{"allowGroupTagManage":1}`, want: roleOwner},
		{name: "只有成员列表权限", permissions: `{"allowGetGroupMembers":1}`, want: roleMember},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher, state := newPermissionDispatcher(t)
			state.setPermissions(tt.permissions)

			payload := memberInfo(t, dispatcher, `{"group_id":20001,"user_id":1001}`)
			if payload["role"] != tt.want {
				t.Errorf("角色为 %v，期望 %s", payload["role"], tt.want)
			}
			if payload["nickname"] != "机器人" {
				t.Errorf("昵称为 %v", payload["nickname"])
			}
			if payload["user_id"] != testSelfID {
				t.Errorf("成员标识为 %v", payload["user_id"])
			}
			// 机器人自己的资料就地构造，不应查询成员列表。
			if calls := state.memberCallCount(); calls != 0 {
				t.Errorf("成员列表接口被调用 %d 次", calls)
			}
		})
	}
}

// TestSelfMemberReportsAdminWhenOwnerDisabled 校验关闭群主上报后，标签权限只算管理员。
func TestSelfMemberReportsAdminWhenOwnerDisabled(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	dispatcher.Tags.ReportOwner = false
	state.setPermissions(`{"allowGroupTagManage":1}`)

	payload := memberInfo(t, dispatcher, `{"group_id":20001,"user_id":1001}`)
	if payload["role"] != roleAdmin {
		t.Errorf("角色为 %v，期望 %s", payload["role"], roleAdmin)
	}
}

func TestSelfMemberWithoutGroupInfo(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.infoFails = true

	// 查不到权限时按普通成员上报，但不让接口失败：框架取不到成员信息会丢弃 @ 段。
	payload := memberInfo(t, dispatcher, `{"group_id":20001,"user_id":1001}`)
	if payload["role"] != roleMember {
		t.Errorf("角色为 %v", payload["role"])
	}
}

func TestOtherMemberKeepsPlatformRole(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.setPermissions(`{"allowGagMember":1}`)

	payload := memberInfo(t, dispatcher, `{"group_id":20001,"user_id":10001}`)
	if payload["role"] != roleAdmin {
		t.Errorf("角色为 %v", payload["role"])
	}
	if payload["nickname"] != "小明" {
		t.Errorf("昵称为 %v", payload["nickname"])
	}
	if calls := state.memberCallCount(); calls != 1 {
		t.Errorf("成员列表接口被调用 %d 次", calls)
	}
}

func TestGroupInfoReportsPermissions(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.setPermissions(`{"allowEditGroupInfo":1,"allowGagMember":0,"allowRemoveMember":1,"allowGroupTagManage":0,"allowGetGroupMembers":1}`)

	value, err := dispatcher.Handle(context.Background(), "get_group_info", json.RawMessage(`{"group_id":20001}`))
	if err != nil {
		t.Fatalf("查询群信息失败: %v", err)
	}

	payload, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("响应类型为 %T", value)
	}
	permissions, ok := payload["yunhu_permissions"].(map[string]any)
	if !ok {
		t.Fatalf("扩展字段类型为 %T", payload["yunhu_permissions"])
	}

	want := map[string]bool{
		"allow_edit_group_info":   true,
		"allow_gag_member":        false,
		"allow_remove_member":     true,
		"allow_group_tag_manage":  false,
		"allow_get_group_members": true,
	}
	for key, expected := range want {
		if permissions[key] != expected {
			t.Errorf("%s 为 %v，期望 %v", key, permissions[key], expected)
		}
	}
}

func TestMemberListDegradedToSelf(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.memberFails = true
	state.setPermissions(`{"allowGagMember":1}`)

	value, err := dispatcher.Handle(context.Background(), "get_group_member_list", json.RawMessage(`{"group_id":20001}`))
	if err != nil {
		t.Fatalf("查询群成员列表失败: %v", err)
	}

	list, ok := value.([]map[string]any)
	if !ok {
		t.Fatalf("响应类型为 %T", value)
	}
	if len(list) != 1 {
		t.Fatalf("列表长度为 %d", len(list))
	}
	if list[0]["user_id"] != testSelfID || list[0]["role"] != roleAdmin {
		t.Errorf("成员为 %v", list[0])
	}
}

func TestMemberListAppendsSelf(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.setPermissions(`{"allowGagMember":1}`)

	value, err := dispatcher.Handle(context.Background(), "get_group_member_list", json.RawMessage(`{"group_id":20001}`))
	if err != nil {
		t.Fatalf("查询群成员列表失败: %v", err)
	}

	list, ok := value.([]map[string]any)
	if !ok {
		t.Fatalf("响应类型为 %T", value)
	}
	if len(list) != 2 {
		t.Fatalf("列表长度为 %d", len(list))
	}
	if list[0]["user_id"] != int64(10001) {
		t.Errorf("第一个成员为 %v", list[0])
	}
	if list[1]["user_id"] != testSelfID || list[1]["role"] != roleAdmin {
		t.Errorf("机器人自己的一条为 %v", list[1])
	}
}

func TestManageFailureExplainsMissingPermission(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.gagFails = true

	params := json.RawMessage(`{"group_id":20001,"user_id":10001,"duration":600}`)
	_, err := dispatcher.Handle(context.Background(), "set_group_ban", params)
	if err == nil {
		t.Fatal("禁言应当失败")
	}

	var actionError *obmodel.ActionError
	if !errors.As(err, &actionError) {
		t.Fatalf("错误类型为 %T", err)
	}
	if actionError.Retcode != obmodel.RetcodeServerError {
		t.Errorf("错误码为 %d", actionError.Retcode)
	}
	if !strings.Contains(actionError.Wording, "allowGagMember") {
		t.Errorf("说明为 %q，应当指出缺少的权限", actionError.Wording)
	}
}

func TestManageFailureWithPermissionStaysUpstream(t *testing.T) {
	dispatcher, state := newPermissionDispatcher(t)
	state.gagFails = true
	// 权限是有的，失败另有原因，说明里不应出现权限二字。
	state.setPermissions(`{"allowGagMember":1}`)

	params := json.RawMessage(`{"group_id":20001,"user_id":10001,"duration":600}`)
	_, err := dispatcher.Handle(context.Background(), "set_group_ban", params)
	if err == nil {
		t.Fatal("禁言应当失败")
	}

	var actionError *obmodel.ActionError
	if errors.As(err, &actionError) {
		t.Fatalf("不应构造权限说明: %q", actionError.Wording)
	}
	if !strings.Contains(err.Error(), "禁言") {
		t.Errorf("错误为 %q", err)
	}
}
