// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Permissions 是机器人在某个群获得的管理权限。
//
// 云湖没有群主、管理员这类身份，机器人的权限由群主或管理员逐项授予，每一项独立
// 开关。0 表示未授权，1 表示已授权。字段名与 GET /group/info 的返回一致。
type Permissions struct {
	// EditGroupInfo 表示允许修改群信息。
	EditGroupInfo bool
	// GagMember 表示允许禁言成员。
	GagMember bool
	// RemoveMember 表示允许移除成员。
	RemoveMember bool
	// GroupTagManage 表示允许管理群标签。
	GroupTagManage bool
	// GetGroupMembers 表示允许读取群成员列表。默认未授权。
	GetGroupMembers bool
}

// CanManage 表示机器人拥有至少一项群管理权限。
//
// 只有写入类权限参与判断，读取成员列表的权限不算管理能力。
func (p Permissions) CanManage() bool {
	return p.EditGroupInfo || p.GagMember || p.RemoveMember || p.GroupTagManage
}

// UnmarshalJSON 解析权限对象。
//
// 平台文档写的是驼峰命名与 0/1 取值，这里同时接受下划线命名与布尔值、字符串
// 写法，避免因为一处字段名差异让整份群信息解析失败。
func (p *Permissions) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return nil
	}

	var value struct {
		AllowEditGroupInfo    flagValue `json:"allowEditGroupInfo"`
		AllowGagMember        flagValue `json:"allowGagMember"`
		AllowRemoveMember     flagValue `json:"allowRemoveMember"`
		AllowGroupTagManage   flagValue `json:"allowGroupTagManage"`
		AllowGetGroupMembers  flagValue `json:"allowGetGroupMembers"`
		AllowEditGroupInfoAlt flagValue `json:"allow_edit_group_info"`
		AllowGagMemberAlt     flagValue `json:"allow_gag_member"`
		AllowRemoveMemberAlt  flagValue `json:"allow_remove_member"`
		AllowGroupTagAlt      flagValue `json:"allow_group_tag_manage"`
		AllowGetMembersAlt    flagValue `json:"allow_get_group_members"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}

	p.EditGroupInfo = bool(value.AllowEditGroupInfo) || bool(value.AllowEditGroupInfoAlt)
	p.GagMember = bool(value.AllowGagMember) || bool(value.AllowGagMemberAlt)
	p.RemoveMember = bool(value.AllowRemoveMember) || bool(value.AllowRemoveMemberAlt)
	p.GroupTagManage = bool(value.AllowGroupTagManage) || bool(value.AllowGroupTagAlt)
	p.GetGroupMembers = bool(value.AllowGetGroupMembers) || bool(value.AllowGetMembersAlt)

	return nil
}

// flagValue 解析 0 与 1 形式的开关，同时接受布尔值与字符串写法。
type flagValue bool

// UnmarshalJSON 解析一个开关取值。
func (f *flagValue) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		value, err := number.Int64()
		if err != nil {
			return fmt.Errorf("开关 %s 不是整数: %w", text, err)
		}
		*f = flagValue(value != 0)
		return nil
	}

	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		*f = flagValue(boolean)
		return nil
	}

	var quoted string
	if err := json.Unmarshal(raw, &quoted); err != nil {
		return fmt.Errorf("开关 %s 既不是数字、布尔值，也不是字符串", text)
	}
	quoted = strings.TrimSpace(quoted)
	*f = flagValue(quoted == "1" || strings.EqualFold(quoted, "true"))

	return nil
}
