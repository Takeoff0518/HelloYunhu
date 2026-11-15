// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"testing"
)

func TestGroupInfoNameField(t *testing.T) {
	// 这段结构与真实响应一致，群名称字段是 name 而不是 groupName。
	raw := []byte(`{"avatarUrl":"https://example.com/a.jpg","groupId":"695367192","headcount":31,"introduction":"U of Vanaheim","name":"瓦纳海姆大学(筹)"}`)

	var info GroupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if info.GroupID != "695367192" {
		t.Errorf("群标识为 %q", info.GroupID)
	}
	if info.GroupName != "瓦纳海姆大学(筹)" {
		t.Errorf("群名称为 %q", info.GroupName)
	}
	if info.Headcount != 31 {
		t.Errorf("成员数量为 %d", info.Headcount)
	}
}

func TestGroupInfoFallbackNameField(t *testing.T) {
	// 旧写法仍要能解析，避免平台同时提供两种字段时行为不一致。
	raw := []byte(`{"groupId":"1","groupName":"旧字段","headcount":2}`)

	var info GroupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if info.GroupName != "旧字段" {
		t.Errorf("群名称为 %q", info.GroupName)
	}
}

func TestGroupSummaryNameField(t *testing.T) {
	raw := []byte(`{"list":[{"avatarUrl":"https://example.com/a.jpg","groupId":"695367192","name":"瓦纳海姆大学(筹)"}]}`)

	var wrapper struct {
		List []GroupSummary `json:"list"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(wrapper.List) != 1 {
		t.Fatalf("群数量为 %d", len(wrapper.List))
	}
	if wrapper.List[0].GroupName != "瓦纳海姆大学(筹)" {
		t.Errorf("群名称为 %q", wrapper.List[0].GroupName)
	}
}

func TestGroupMemberToleratesFieldNames(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "userId", raw: `{"userId":"10001","nickname":"甲"}`, want: "10001"},
		{name: "senderId", raw: `{"senderId":"10002","nickName":"乙"}`, want: "10002"},
		{name: "id", raw: `{"id":"10003","nickname":"丙"}`, want: "10003"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var member GroupMember
			if err := json.Unmarshal([]byte(tt.raw), &member); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if member.UserID != tt.want {
				t.Errorf("成员标识为 %q", member.UserID)
			}
			if member.Nickname == "" {
				t.Error("昵称不应为空")
			}
		})
	}
}
