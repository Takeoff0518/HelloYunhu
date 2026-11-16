// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// TagOptions 是群标签相关的行为选项，由配置映射得到。
type TagOptions struct {
	// AutoCreate 表示设置头衔时标签不存在就自动创建。
	AutoCreate bool
	// Color 是自动创建标签时使用的颜色，格式为 #RRGGBB。
	Color string
	// Replace 表示设置头衔时解绑该用户的其余标签。
	Replace bool
	// ClearOnEmpty 表示头衔置空时解绑该用户的全部标签。
	ClearOnEmpty bool
	// ReportOwner 表示持有标签管理权限的机器人按群主上报。
	//
	// QQ 里设置专属头衔是群主专属操作。关掉之后这类机器人按管理员上报，只认群主
	// 的插件会继续拒绝设置头衔。
	ReportOwner bool
}

// tagParams 是标签类接口的公共参数。
//
// Desc 与 Sort 用指针接住，调用方才能区分「不改这一项」与「改成空字符串或 0」。
type tagParams struct {
	GroupID int64   `json:"group_id"`
	UserID  int64   `json:"user_id"`
	Tag     string  `json:"tag"`
	NewTag  string  `json:"new_tag"`
	Color   string  `json:"color"`
	Desc    *string `json:"desc"`
	Sort    *int64  `json:"sort"`
}

// groupTags 处理 yunhu_get_group_tags。
//
// 标签列表接口只要求机器人在群中，不要求标签管理权限。
func (d *Dispatcher) groupTags(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_get_group_tags"

	var value struct {
		GroupID int64 `json:"group_id"`
	}
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest(action, err)
	}
	if value.GroupID <= 0 {
		return nil, missingGroupID(action)
	}

	tags, err := d.API.GroupTags(ctx, strconv.FormatInt(value.GroupID, 10))
	if err != nil {
		return nil, fmt.Errorf("查询群标签: %w", err)
	}

	return tagList(tags), nil
}

// createGroupTag 处理 yunhu_create_group_tag。
func (d *Dispatcher) createGroupTag(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_create_group_tag"

	value, err := parseTagParams(action, params)
	if err != nil {
		return nil, err
	}

	request := yunhumodel.CreateTagRequest{
		GroupID: strconv.FormatInt(value.GroupID, 10),
		Tag:     value.Tag,
		Sort:    value.Sort,
	}
	if value.Desc != nil {
		request.Desc = *value.Desc
	}
	if color := strings.ToUpper(strings.TrimSpace(value.Color)); color != "" {
		if !validTagColor(color) {
			return nil, invalidColor(action, color)
		}
		request.Color = color
	}

	if err := d.API.CreateGroupTag(ctx, request); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "创建群标签", err)
	}

	return map[string]any{}, nil
}

// editGroupTag 处理 yunhu_edit_group_tag。
func (d *Dispatcher) editGroupTag(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_edit_group_tag"

	value, err := parseTagParams(action, params)
	if err != nil {
		return nil, err
	}

	request := yunhumodel.EditTagRequest{
		GroupID: strconv.FormatInt(value.GroupID, 10),
		Tag:     value.Tag,
		Desc:    value.Desc,
		Sort:    value.Sort,
	}

	if strings.TrimSpace(value.NewTag) != "" {
		name, err := normalizeTagName(action, value.NewTag)
		if err != nil {
			return nil, err
		}
		request.NewTag = name
	}
	if color := strings.ToUpper(strings.TrimSpace(value.Color)); color != "" {
		if !validTagColor(color) {
			return nil, invalidColor(action, color)
		}
		request.Color = color
	}

	// 一项都不给时调用没有意义，报错比发一个空修改更容易发现调用方的问题。
	if request.NewTag == "" && request.Color == "" && request.Desc == nil && request.Sort == nil {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: action + " 至少要给出 new_tag、color、desc、sort 中的一项",
		}
	}

	if err := d.API.EditGroupTag(ctx, request); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "修改群标签", err)
	}

	return map[string]any{}, nil
}

// deleteGroupTag 处理 yunhu_delete_group_tag。
func (d *Dispatcher) deleteGroupTag(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_delete_group_tag"

	value, err := parseTagParams(action, params)
	if err != nil {
		return nil, err
	}

	if err := d.API.DeleteGroupTag(ctx, strconv.FormatInt(value.GroupID, 10), value.Tag); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "删除群标签", err)
	}

	return map[string]any{}, nil
}

// addGroupMemberTag 处理 yunhu_add_group_member_tag。
//
// 这个接口只加不删，标签不存在时报错，不自动创建。
func (d *Dispatcher) addGroupMemberTag(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_add_group_member_tag"

	value, err := parseTagParams(action, params)
	if err != nil {
		return nil, err
	}
	if value.UserID <= 0 {
		return nil, missingUserID(action)
	}

	gid := strconv.FormatInt(value.GroupID, 10)
	uid := strconv.FormatInt(value.UserID, 10)
	if err := d.API.AddMemberTag(ctx, gid, uid, value.Tag); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "给成员加标签", err)
	}

	return map[string]any{}, nil
}

// removeGroupMemberTag 处理 yunhu_remove_group_member_tag。
func (d *Dispatcher) removeGroupMemberTag(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "yunhu_remove_group_member_tag"

	value, err := parseTagParams(action, params)
	if err != nil {
		return nil, err
	}
	if value.UserID <= 0 {
		return nil, missingUserID(action)
	}

	gid := strconv.FormatInt(value.GroupID, 10)
	uid := strconv.FormatInt(value.UserID, 10)
	if err := d.API.RemoveMemberTag(ctx, gid, uid, value.Tag); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "移除成员标签", err)
	}

	return map[string]any{}, nil
}

// setGroupSpecialTitle 处理 set_group_special_title。
//
// 云湖的群标签与 QQ 的专属头衔效果相同，区别是一个成员可以有多个标签。标准接口
// 只给一个字符串，因此按配置决定语义：replace 表示设置后只剩这一个标签，append
// 表示只追加。置空时的行为由 clear_on_empty 决定。
func (d *Dispatcher) setGroupSpecialTitle(ctx context.Context, params json.RawMessage) (any, error) {
	const action = "set_group_special_title"

	var value struct {
		GroupID      int64  `json:"group_id"`
		UserID       int64  `json:"user_id"`
		SpecialTitle string `json:"special_title"`
		Duration     int    `json:"duration"`
	}
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest(action, err)
	}
	if value.GroupID <= 0 {
		return nil, missingGroupID(action)
	}
	if value.UserID <= 0 {
		return nil, missingUserID(action)
	}

	title := strings.TrimSpace(value.SpecialTitle)
	if title == "" {
		return d.clearSpecialTitle(ctx, value.GroupID, value.UserID)
	}

	name, err := normalizeTagName(action, title)
	if err != nil {
		return nil, err
	}
	if value.Duration != 0 {
		d.logger().Debug("云湖的标签没有时长概念，duration 已忽略",
			"group_id", value.GroupID, "user_id", value.UserID, "duration", value.Duration)
	}

	gid := strconv.FormatInt(value.GroupID, 10)
	tags, err := d.API.GroupTags(ctx, gid)
	if err != nil {
		return nil, fmt.Errorf("查询群标签: %w", err)
	}

	if !hasTag(tags, name) {
		if !d.Tags.AutoCreate {
			return nil, tagMissingError(value.GroupID, name, tags)
		}
		request := yunhumodel.CreateTagRequest{GroupID: gid, Tag: name, Color: d.Tags.Color}
		if err := d.API.CreateGroupTag(ctx, request); err != nil {
			return nil, d.explainTagFailure(ctx, value.GroupID, "设置头衔", err)
		}
		d.logger().Info("已自动创建群标签", "group_id", value.GroupID, "tag", name)
	}

	uid := strconv.FormatInt(value.UserID, 10)
	if err := d.API.AddMemberTag(ctx, gid, uid, name); err != nil {
		return nil, d.explainTagFailure(ctx, value.GroupID, "设置头衔", err)
	}

	if !d.Tags.Replace {
		return map[string]any{}, nil
	}

	// 先加后删：目标标签已经绑上，再解绑其余标签，用户不会出现没有头衔的中间态。
	if err := d.unbindMemberTags(ctx, value.GroupID, value.UserID, name, tags); err != nil {
		return nil, err
	}

	return map[string]any{}, nil
}

// clearSpecialTitle 解绑该用户在群里的全部标签。
//
// 程序读不到用户身上有哪些标签，只能把群标签列表逐个解绑。
func (d *Dispatcher) clearSpecialTitle(ctx context.Context, groupID, userID int64) (any, error) {
	if !d.Tags.ClearOnEmpty {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeServerError,
			Wording: "头衔清空已在配置里关闭（tags.special_title.clear_on_empty），本次未做修改",
		}
	}

	tags, err := d.API.GroupTags(ctx, strconv.FormatInt(groupID, 10))
	if err != nil {
		return nil, fmt.Errorf("查询群标签: %w", err)
	}
	if len(tags) == 0 {
		return map[string]any{}, nil
	}

	if err := d.unbindMemberTags(ctx, groupID, userID, "", tags); err != nil {
		return nil, err
	}

	return map[string]any{}, nil
}

// unbindMemberTags 解绑该用户在群里的标签，keep 非空时跳过同名标签。
//
// 单个标签失败不中断：解绑一个本来就没有的标签很可能被平台拒绝，这种情况不该让
// 其余标签留在用户身上。全部失败时返回错误，那多半是权限问题。
func (d *Dispatcher) unbindMemberTags(ctx context.Context, groupID, userID int64, keep string, tags []yunhumodel.GroupTag) error {
	gid := strconv.FormatInt(groupID, 10)
	uid := strconv.FormatInt(userID, 10)

	removed := 0
	var firstErr error
	for _, tag := range tags {
		if tag.Tag == keep {
			continue
		}
		if err := d.API.RemoveMemberTag(ctx, gid, uid, tag.Tag); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			d.logger().Warn("解绑群标签失败，已跳过",
				"group_id", groupID, "user_id", userID, "tag", tag.Tag, "err", err)
			continue
		}
		removed++
	}

	if removed == 0 && firstErr != nil {
		return fmt.Errorf("解绑群标签: %w", firstErr)
	}
	if firstErr != nil {
		d.logger().Warn("部分群标签未能解绑",
			"group_id", groupID, "user_id", userID, "removed", removed)
	}

	return nil
}

// explainTagFailure 在标签操作失败后确认机器人是否缺少标签管理权限。
func (d *Dispatcher) explainTagFailure(ctx context.Context, groupID int64, action string, err error) error {
	return d.explainPermission(ctx, groupID, action, "allowGroupTagManage",
		func(p yunhumodel.Permissions) bool { return p.GroupTagManage }, err)
}

// tagMissingError 说明群内没有目标标签，并给出可用的标签。
func tagMissingError(groupID int64, name string, tags []yunhumodel.GroupTag) error {
	if len(tags) == 0 {
		return &obmodel.ActionError{
			Retcode: obmodel.RetcodeServerError,
			Wording: fmt.Sprintf("群 %d 还没有标签，可以开启 tags.auto_create 自动创建，"+
				"或先用 yunhu_create_group_tag 建一个", groupID),
		}
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Tag)
	}

	return &obmodel.ActionError{
		Retcode: obmodel.RetcodeServerError,
		Wording: fmt.Sprintf("群 %d 没有名为「%s」的标签，现有标签：%s", groupID, name, strings.Join(names, "、")),
	}
}

// tagList 把标签定义转换成扩展接口的返回结构。
func tagList(tags []yunhumodel.GroupTag) []map[string]any {
	list := make([]map[string]any, 0, len(tags))
	for _, tag := range tags {
		list = append(list, map[string]any{
			"tag":   tag.Tag,
			"desc":  tag.Desc,
			"color": tag.Color,
			"sort":  tag.Sort,
		})
	}
	return list
}

// hasTag 判断标签列表里是否有指定名称。
func hasTag(tags []yunhumodel.GroupTag, name string) bool {
	for _, tag := range tags {
		if tag.Tag == name {
			return true
		}
	}
	return false
}

// parseTagParams 解析标签类接口的参数，并校验群标识与标签名称。
func parseTagParams(action string, params json.RawMessage) (tagParams, error) {
	var value tagParams
	if err := json.Unmarshal(params, &value); err != nil {
		return tagParams{}, badRequest(action, err)
	}
	if value.GroupID <= 0 {
		return tagParams{}, missingGroupID(action)
	}

	name, err := normalizeTagName(action, value.Tag)
	if err != nil {
		return tagParams{}, err
	}
	value.Tag = name

	return value, nil
}

// normalizeTagName 去掉首尾空白并校验标签名称的长度。
func normalizeTagName(action, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: action + " 缺少 tag"}
	}
	if count := utf8.RuneCountInString(name); count > yunhumodel.MaxTagRunes {
		return "", &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: fmt.Sprintf("%s 的标签名最长 %d 个字符，当前 %d 个",
				action, yunhumodel.MaxTagRunes, count),
		}
	}

	return name, nil
}

// validTagColor 判断颜色是否为 #RRGGBB 形式。
func validTagColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, char := range color[1:] {
		switch {
		case char >= '0' && char <= '9', char >= 'A' && char <= 'F', char >= 'a' && char <= 'f':
		default:
			return false
		}
	}
	return true
}

// invalidColor 返回颜色格式错误的说明。
func invalidColor(action, color string) *obmodel.ActionError {
	return &obmodel.ActionError{
		Retcode: obmodel.RetcodeBadRequest,
		Wording: fmt.Sprintf("%s 的 color 需要写成 #RRGGBB，当前为 %q", action, color),
	}
}

// missingUserID 返回缺少成员标识的错误。
func missingUserID(action string) *obmodel.ActionError {
	return &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: action + " 缺少 user_id"}
}
