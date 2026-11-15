// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// Recall 撤回一条消息。
//
// 云湖的撤回接口用 chatType 区分会话，取值是 user 与 group，与事件的 bot 与 group
// 不一致，调用前统一换算。撤回接口实测可用，但平台不会推送撤回通知。
func (c *Client) Recall(ctx context.Context, chatType, chatID, msgID string) error {
	if msgID == "" {
		return errors.New("撤回消息缺少消息标识")
	}
	if chatID == "" {
		return errors.New("撤回消息缺少会话标识")
	}

	normalized, err := recallChatType(chatType)
	if err != nil {
		return err
	}

	body := map[string]any{
		"msgId":    msgID,
		"chatId":   chatID,
		"chatType": normalized,
	}
	if _, err := c.do(ctx, http.MethodPost, "/bot/recall", "POST /bot/recall", nil, body); err != nil {
		return err
	}

	return nil
}

// recallChatType 把事件里的会话类型换算成撤回接口的取值。
func recallChatType(chatType string) (string, error) {
	switch chatType {
	case model.ChatTypeGroup:
		return "group", nil
	case model.ChatTypeBot, model.RecvTypeUser:
		return "user", nil
	default:
		return "", fmt.Errorf("撤回消息的会话类型 %q 不受支持", chatType)
	}
}

// GagMember 禁言群成员。
//
// duration 的单位是秒，0 表示解除禁言，-1 表示永久。平台要求群主或管理员给机器人
// 开启禁言权限，没有权限时返回业务错误，调用方原样上报。
func (c *Client) GagMember(ctx context.Context, groupID, userID string, duration int) error {
	if groupID == "" || userID == "" {
		return errors.New("禁言缺少群标识或成员标识")
	}

	body := map[string]any{
		"groupId": groupID,
		"userId":  userID,
		"gag":     duration,
	}
	if _, err := c.do(ctx, http.MethodPost, "/group/gag-member", "POST /group/gag-member", nil, body); err != nil {
		return err
	}

	return nil
}

// RemoveMember 把成员移出群。
//
// 平台要求机器人拥有移除成员的权限；OneBot 的 reject_add_request 在云湖没有对应
// 参数，由调用方忽略。
func (c *Client) RemoveMember(ctx context.Context, groupID, userID string) error {
	if groupID == "" || userID == "" {
		return errors.New("移除成员缺少群标识或成员标识")
	}

	body := map[string]any{
		"groupId": groupID,
		"userId":  userID,
	}
	if _, err := c.do(ctx, http.MethodPost, "/group/remove-member", "POST /group/remove-member", nil, body); err != nil {
		return err
	}

	return nil
}
