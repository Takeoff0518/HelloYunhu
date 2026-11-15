// SPDX-License-Identifier: GPL-3.0-or-later

// Package action 实现框架发来的 OneBot 接口调用。
package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// deleteParams 是 delete_msg 的参数。
type deleteParams struct {
	MessageID obmodel.IDValue `json:"message_id"`
}

// manageParams 是群管理类接口的参数。
type manageParams struct {
	GroupID          int64 `json:"group_id"`
	UserID           int64 `json:"user_id"`
	Duration         int   `json:"duration"`
	RejectAddRequest bool  `json:"reject_add_request"`
}

// deleteMessage 处理 delete_msg，按会话信息调用撤回接口。
//
// OneBot 只给消息标识，云湖还需要会话标识与类型，因此依赖消息归属缓存。缓存是进程
// 内的，重启后撤回旧消息会返回 1404，而不是猜一个会话。
func (d *Dispatcher) deleteMessage(ctx context.Context, params json.RawMessage) (any, error) {
	var value deleteParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("delete_msg", err)
	}

	messageID := value.MessageID.Int64()
	if messageID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "delete_msg 缺少 message_id"}
	}

	origin, ok := d.Origins.Get(messageID)
	if !ok {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeNotFound,
			Wording: fmt.Sprintf("消息 %d 不在最近的记录中，可能是重启前发收的消息", messageID),
		}
	}

	msgID, ok, err := d.Messages.MsgID(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("反查消息标识: %w", err)
	}
	if !ok {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeNotFound,
			Wording: fmt.Sprintf("消息 %d 没有对应的云湖标识", messageID),
		}
	}

	if err := d.API.Recall(ctx, origin.ChatType, origin.ChatID, msgID); err != nil {
		return nil, fmt.Errorf("撤回消息: %w", err)
	}

	d.logger().Debug("已撤回消息", "message_id", messageID, "chat_type", origin.ChatType)
	return map[string]any{}, nil
}

// setGroupBan 处理 set_group_ban。
//
// duration 为 0 时解除禁言，为负数时按永久禁言处理（云湖用 -1 表示永久）。
func (d *Dispatcher) setGroupBan(ctx context.Context, params json.RawMessage) (any, error) {
	value, err := d.parseManageParams("set_group_ban", params)
	if err != nil {
		return nil, err
	}

	if err := d.API.GagMember(ctx, strconv.FormatInt(value.GroupID, 10), strconv.FormatInt(value.UserID, 10), value.Duration); err != nil {
		return nil, fmt.Errorf("禁言成员: %w", err)
	}

	d.logger().Debug("已设置禁言", "group_id", value.GroupID, "user_id", value.UserID, "duration", value.Duration)
	return map[string]any{}, nil
}

// setGroupKick 处理 set_group_kick。
//
// 云湖的移除成员接口没有「拒绝再加群」的参数，reject_add_request 只能忽略。
func (d *Dispatcher) setGroupKick(ctx context.Context, params json.RawMessage) (any, error) {
	value, err := d.parseManageParams("set_group_kick", params)
	if err != nil {
		return nil, err
	}

	if value.RejectAddRequest {
		d.logger().Debug("云湖没有拒绝再加群的参数，已忽略", "group_id", value.GroupID, "user_id", value.UserID)
	}

	if err := d.API.RemoveMember(ctx, strconv.FormatInt(value.GroupID, 10), strconv.FormatInt(value.UserID, 10)); err != nil {
		return nil, fmt.Errorf("移除群成员: %w", err)
	}

	d.logger().Debug("已移出群成员", "group_id", value.GroupID, "user_id", value.UserID)
	return map[string]any{}, nil
}

// parseManageParams 解析群管理类接口的参数。
func (d *Dispatcher) parseManageParams(action string, params json.RawMessage) (manageParams, error) {
	var value manageParams
	if err := json.Unmarshal(params, &value); err != nil {
		return manageParams{}, badRequest(action, err)
	}
	if value.GroupID <= 0 {
		return manageParams{}, missingGroupID(action)
	}
	if value.UserID <= 0 {
		return manageParams{}, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: action + " 缺少 user_id",
		}
	}

	return value, nil
}
