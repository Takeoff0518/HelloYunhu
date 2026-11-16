// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"testing"
)

func TestPermissionsParseNumbers(t *testing.T) {
	// 字段名与取值取自 GET /group/info 的文档与示例。
	raw := []byte(`{"allowEditGroupInfo":1,"allowGagMember":0,"allowRemoveMember":1,"allowGroupTagManage":0,"allowGetGroupMembers":1}`)

	var value Permissions
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if !value.EditGroupInfo || value.GagMember || !value.RemoveMember || value.GroupTagManage || !value.GetGroupMembers {
		t.Errorf("权限为 %+v", value)
	}
}

func TestPermissionsTolerateBoolAndSnakeCase(t *testing.T) {
	raw := []byte(`{"allow_gag_member":true,"allow_remove_member":"1","allowGetGroupMembers":"false"}`)

	var value Permissions
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if !value.GagMember || !value.RemoveMember {
		t.Errorf("权限为 %+v", value)
	}
	if value.GetGroupMembers {
		t.Errorf("字符串 false 应当解析为未授权: %+v", value)
	}
}

func TestPermissionsNullIsZero(t *testing.T) {
	raw := []byte(`{"permissions":null}`)

	var wrapper struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if wrapper.Permissions.CanManage() {
		t.Errorf("空权限不应具备管理能力: %+v", wrapper.Permissions)
	}
}

func TestPermissionsCanManage(t *testing.T) {
	tests := []struct {
		name  string
		value Permissions
		want  bool
	}{
		{name: "全未授权", value: Permissions{}, want: false},
		{name: "只有成员列表权限", value: Permissions{GetGroupMembers: true}, want: false},
		{name: "禁言", value: Permissions{GagMember: true}, want: true},
		{name: "移除成员", value: Permissions{RemoveMember: true}, want: true},
		{name: "修改群信息", value: Permissions{EditGroupInfo: true}, want: true},
		{name: "群标签", value: Permissions{GroupTagManage: true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.CanManage(); got != tt.want {
				t.Errorf("CanManage 为 %v，期望 %v", got, tt.want)
			}
		})
	}
}

func TestGroupInfoCarriesPermissions(t *testing.T) {
	raw := []byte(`{"groupId":"695367192","name":"测试群","headcount":31,"permissions":{"allowGagMember":1,"allowGetGroupMembers":0}}`)

	var info GroupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if !info.Permissions.GagMember {
		t.Error("群信息没有带上禁言权限")
	}
	if !info.Permissions.CanManage() {
		t.Error("持有禁言权限时应当具备管理能力")
	}
}
