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

// 群成员角色取值。OneBot 只有这三项，云湖的权限开关需要换算成其中之一。
const (
	roleOwner  = "owner"
	roleAdmin  = "admin"
	roleMember = "member"
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
// 机器人自己的资料就地构造，不查询成员列表：标识与昵称在启动时已经取得，权限来自
// 群信息，因此成员列表权限未开启时也能回答。这一点很重要，框架在取不到成员信息时
// 会丢弃 @ 消息段，导致群里 @ 机器人无法唤醒。
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

	if value.UserID == d.SelfID {
		return d.selfMember(ctx, value.GroupID), nil
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
		d.rememberAvatar(member)
		return memberPayload(value.GroupID, value.UserID, member, normalizeRole(member.Role)), nil
	}

	// 列表里没有这个人，同样返回空数据，让框架走补空昵称的分支。
	return map[string]any{}, nil
}

// groupMemberList 处理 get_group_member_list。
//
// 群成员权限默认关闭，没有权限时退化为只返回机器人自己：这一条资料是本程序确定
// 的，比空列表有用，按列表查找机器人的插件还能继续工作。
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
		d.logger().Warn("查询群成员列表失败，退化为只返回机器人自己", "group_id", value.GroupID, "err", err)
		return []map[string]any{d.selfMember(ctx, value.GroupID)}, nil
	}

	list := make([]map[string]any, 0, len(members))
	seenSelf := false
	selfRole := ""
	for _, member := range members {
		userID, _ := identity.ParseID(member.UserID)
		if userID <= 0 {
			continue
		}
		d.rememberAvatar(member)

		role := normalizeRole(member.Role)
		if userID == d.SelfID {
			// 平台按成员角色给出的取值对机器人没有意义，改用权限推导的结果。
			if selfRole == "" {
				selfRole = d.selfRole(ctx, value.GroupID)
			}
			role = selfRole
			seenSelf = true
		}

		list = append(list, memberPayload(value.GroupID, userID, member, role))
	}

	// 成员列表通常不包含机器人，补上一条，插件遍历列表查找机器人时才有结果。
	if !seenSelf {
		list = append(list, d.selfMember(ctx, value.GroupID))
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

// selfMember 构造机器人在群里的成员信息。
//
// 不调用成员列表接口，也不依赖成员列表权限。
func (d *Dispatcher) selfMember(ctx context.Context, groupID int64) map[string]any {
	member := model.GroupMember{
		UserID:   strconv.FormatInt(d.SelfID, 10),
		Nickname: d.Nickname,
	}

	return memberPayload(groupID, d.SelfID, member, d.selfRole(ctx, groupID))
}

// selfRole 推导机器人在群里的角色。
//
// 云湖没有群主与管理员这类身份，机器人拿到的是逐项授予的管理权限。持有标签管理
// 权限时按群主上报：QQ 里设置专属头衔是群主专属操作，云湖把它单独拆成一个开关，
// 拿到开关的机器人在这件事上等价于群主，按管理员上报会让只认群主的插件继续拒绝。
// 其余权限按管理员上报。
//
// 每次调用都查询群信息，不做缓存：群主调整权限的时机常常就是插件报告权限不足
// 之后，缓存会让刚授予的权限继续被拒绝。
func (d *Dispatcher) selfRole(ctx context.Context, groupID int64) string {
	if d.API == nil {
		return roleMember
	}

	info, err := d.API.GroupInfo(ctx, strconv.FormatInt(groupID, 10))
	if err != nil {
		d.logger().Warn("查询群权限失败，机器人角色按普通成员上报", "group_id", groupID, "err", err)
		return roleMember
	}
	if d.Tags.ReportOwner && info.Permissions.GroupTagManage {
		return roleOwner
	}
	if info.Permissions.CanManage() {
		return roleAdmin
	}

	return roleMember
}

// groupPayload 构造 OneBot 的群信息。
//
// yunhu_permissions 是本程序新增的扩展字段，标准客户端会忽略它。
func groupPayload(groupID int64, info *model.GroupInfo) map[string]any {
	return map[string]any{
		"group_id":          groupID,
		"group_name":        info.GroupName,
		"member_count":      info.Headcount,
		"max_member_count":  info.Headcount,
		"yunhu_permissions": permissionPayload(info.Permissions),
	}
}

// permissionPayload 把权限开关转换成扩展字段的取值。
func permissionPayload(value model.Permissions) map[string]any {
	return map[string]any{
		"allow_edit_group_info":   value.EditGroupInfo,
		"allow_gag_member":        value.GagMember,
		"allow_remove_member":     value.RemoveMember,
		"allow_group_tag_manage":  value.GroupTagManage,
		"allow_get_group_members": value.GetGroupMembers,
	}
}

// memberPayload 构造 OneBot 的群成员信息。
func memberPayload(groupID, userID int64, member model.GroupMember, role string) map[string]any {
	return map[string]any{
		"group_id": groupID,
		"user_id":  userID,
		"nickname": member.Nickname,
		"card":     member.Card,
		"role":     role,
	}
}

// normalizeRole 把云湖的成员角色收敛到 OneBot 的三个取值。
func normalizeRole(level string) string {
	switch level {
	case roleOwner, roleAdmin:
		return level
	default:
		return roleMember
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
