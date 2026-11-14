// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// groupParams 是群类接口的公共参数。
type groupParams struct {
	GroupID int64 `json:"group_id"`
	UserID  int64 `json:"user_id"`
}

// groupInfo 处理 get_group_info。
func (d *Dispatcher) groupInfo(ctx context.Context, params json.RawMessage) (any, error) {
	value, err := parseGroupParams(params)
	if err != nil {
		return nil, badRequest("get_group_info", err)
	}
	if value.GroupID <= 0 {
		return nil, missingGroupID("get_group_info")
	}

	info, err := d.API.GroupInfo(ctx, strconv.FormatInt(value.GroupID, 10))
	if err != nil {
		return nil, fmt.Errorf("查询群信息: %w", err)
	}

	return groupPayload(value.GroupID, info), nil
}

// groupList 处理 get_group_list。
func (d *Dispatcher) groupList(ctx context.Context) (any, error) {
	groups, err := d.API.BotGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询群列表: %w", err)
	}

	list := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		groupID, _ := identity.ParseID(group.GroupID)
		if groupID <= 0 {
			continue
		}
		list = append(list, map[string]any{
			"group_id":         groupID,
			"group_name":       group.GroupName,
			"member_count":     group.Headcount,
			"max_member_count": group.Headcount,
		})
	}

	return list, nil
}

// groupMemberInfo 处理 get_group_member_info。
//
// 群成员权限默认关闭，平台会返回业务错误。框架在取不到成员信息时会丢弃 @ 消息段，
// 导致群里 @ 机器人无法唤醒，因此这里即使失败也返回成功与空数据。
func (d *Dispatcher) groupMemberInfo(ctx context.Context, params json.RawMessage) (any, error) {
	value, err := parseGroupParams(params)
	if err != nil {
		return nil, badRequest("get_group_member_info", err)
	}
	if value.GroupID <= 0 {
		return nil, missingGroupID("get_group_member_info")
	}
	if value.UserID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "get_group_member_info 缺少 user_id"}
	}

	members, err := d.API.GroupMembers(ctx, strconv.FormatInt(value.GroupID, 10))
	if err != nil {
		d.logger().Warn("查询群成员失败，返回空数据以避免框架丢弃 @ 消息段", "group_id", value.GroupID, "err", err)
		return map[string]any{}, nil
	}

	for _, member := range members {
		if member.UserID != strconv.FormatInt(value.UserID, 10) {
			continue
		}
		return memberPayload(value.GroupID, value.UserID, member), nil
	}

	// 列表里没有这个人，同样返回空数据，让框架走补空昵称的分支。
	return map[string]any{}, nil
}

// groupMemberList 处理 get_group_member_list。
func (d *Dispatcher) groupMemberList(ctx context.Context, params json.RawMessage) (any, error) {
	value, err := parseGroupParams(params)
	if err != nil {
		return nil, badRequest("get_group_member_list", err)
	}
	if value.GroupID <= 0 {
		return nil, missingGroupID("get_group_member_list")
	}

	members, err := d.API.GroupMembers(ctx, strconv.FormatInt(value.GroupID, 10))
	if err != nil {
		d.logger().Warn("查询群成员列表失败，返回空列表", "group_id", value.GroupID, "err", err)
		return []any{}, nil
	}

	list := make([]map[string]any, 0, len(members))
	for _, member := range members {
		userID, _ := identity.ParseID(member.UserID)
		if userID <= 0 {
			continue
		}
		list = append(list, memberPayload(value.GroupID, userID, member))
	}

	return list, nil
}

// strangerInfo 处理 get_stranger_info。
//
// 云湖没有陌生人信息接口，返回空昵称，框架可以正常继续。
func (d *Dispatcher) strangerInfo(ctx context.Context, params json.RawMessage) (any, error) {
	var value groupParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("get_stranger_info", err)
	}
	if value.UserID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "get_stranger_info 缺少 user_id"}
	}

	return map[string]any{"user_id": value.UserID, "nickname": ""}, nil
}

// groupPayload 构造 OneBot 的群信息。
func groupPayload(groupID int64, info *model.GroupInfo) map[string]any {
	return map[string]any{
		"group_id":         groupID,
		"group_name":       info.GroupName,
		"member_count":     info.Headcount,
		"max_member_count": info.Headcount,
	}
}

// memberPayload 构造 OneBot 的群成员信息。
func memberPayload(groupID, userID int64, member model.GroupMember) map[string]any {
	role := member.Role
	switch role {
	case "owner", "admin":
	default:
		role = "member"
	}

	return map[string]any{
		"group_id": groupID,
		"user_id":  userID,
		"nickname": member.Nickname,
		"card":     member.Card,
		"role":     role,
	}
}

// parseGroupParams 解析群类接口的参数。参数可能为空，按缺省处理。
func parseGroupParams(params json.RawMessage) (groupParams, error) {
	var value groupParams
	if len(params) == 0 || string(params) == "null" {
		return value, nil
	}
	if err := json.Unmarshal(params, &value); err != nil {
		return groupParams{}, err
	}
	return value, nil
}

// missingGroupID 返回缺少群标识的错误。
func missingGroupID(action string) *obmodel.ActionError {
	return &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: action + " 缺少 group_id"}
}
