// SPDX-License-Identifier: GPL-3.0-or-later

// Package action 实现框架发来的 OneBot 接口调用。
//
// 每个接口把 OneBot 的参数翻译成云湖的调用，再按 OneBot 的格式返回。未实现的
// 接口返回 1404 与说明，不返回成功；参数不合法返回 100；上游失败原样返回错误，
// 由连接层包装成 failed 响应。
package action

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/message"
	"github.com/Takeoff0518/HelloYunhu/internal/logging"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// Dispatcher 是 OneBot 接口调用的处理器。
type Dispatcher struct {
	// SelfID 是机器人自身的整数标识。
	SelfID int64
	// Nickname 是机器人昵称。
	Nickname string
	// API 是云湖接口客户端。
	API *api.Client
	// Messages 是消息标识映射。
	Messages identity.MessageStore
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
}

// Handle 处理一次接口调用，返回值会写进响应的 data 字段。
func (d *Dispatcher) Handle(ctx context.Context, name string, params json.RawMessage) (any, error) {
	switch name {
	case "send_group_msg":
		return d.sendGroupMessage(ctx, params)
	case "send_private_msg":
		return d.sendPrivateMessage(ctx, params)
	case "send_msg":
		return d.sendMessage(ctx, params)
	case "get_login_info":
		return map[string]any{"user_id": d.SelfID, "nickname": d.Nickname}, nil
	case "get_status":
		return map[string]any{"online": true, "good": true}, nil
	case "get_version_info":
		return map[string]any{"app_name": "HelloYunhu", "protocol_version": "v11"}, nil
	case ".handle_quick_operation", ".handle_quick_operation_async":
		// aiocqhttp 处理快速操作后会回调这个接口，返回成功即可。
		return map[string]any{}, nil
	default:
		return nil, obmodel.NotImplemented(name)
	}
}

// sendParams 是发送类接口的公共参数。
//
// 框架会把 self_id 等路由参数一并放进 params，这里只取需要的字段，多余参数忽略。
type sendParams struct {
	MessageType string            `json:"message_type"`
	GroupID     int64             `json:"group_id"`
	UserID      int64             `json:"user_id"`
	Message     obmodel.InMessage `json:"message"`
}

// sendGroupMessage 处理 send_group_msg。
func (d *Dispatcher) sendGroupMessage(ctx context.Context, params json.RawMessage) (any, error) {
	var value sendParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("send_group_msg", err)
	}
	if value.GroupID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_group_msg 缺少 group_id"}
	}

	return d.send(ctx, yunhumodel.RecvTypeGroup, strconv.FormatInt(value.GroupID, 10), value.Message)
}

// sendPrivateMessage 处理 send_private_msg。
func (d *Dispatcher) sendPrivateMessage(ctx context.Context, params json.RawMessage) (any, error) {
	var value sendParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("send_private_msg", err)
	}
	if value.UserID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_private_msg 缺少 user_id"}
	}

	return d.send(ctx, yunhumodel.RecvTypeUser, strconv.FormatInt(value.UserID, 10), value.Message)
}

// sendMessage 处理 send_msg，按 message_type 决定接收方。
func (d *Dispatcher) sendMessage(ctx context.Context, params json.RawMessage) (any, error) {
	var value sendParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("send_msg", err)
	}

	switch value.MessageType {
	case obmodel.MessageTypeGroup:
		if value.GroupID <= 0 {
			return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_msg 缺少 group_id"}
		}
		return d.send(ctx, yunhumodel.RecvTypeGroup, strconv.FormatInt(value.GroupID, 10), value.Message)
	case obmodel.MessageTypePrivate:
		if value.UserID <= 0 {
			return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_msg 缺少 user_id"}
		}
		return d.send(ctx, yunhumodel.RecvTypeUser, strconv.FormatInt(value.UserID, 10), value.Message)
	default:
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: fmt.Sprintf("send_msg 的 message_type %q 不受支持", value.MessageType),
		}
	}
}

// send 把消息段整理成云湖文本消息并发送。
func (d *Dispatcher) send(ctx context.Context, recvType, recvID string, segments obmodel.InMessage) (any, error) {
	if d.API == nil {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeServerError, Wording: "接口客户端尚未初始化"}
	}

	content, err := message.FromOneBot(segments)
	if err != nil {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: err.Error()}
	}

	text := content.Text
	request := yunhumodel.SendMessageRequest{
		RecvID:      recvID,
		RecvType:    recvType,
		ContentType: yunhumodel.ContentTypeText,
		Content:     map[string]any{"text": text},
		ParentID:    d.parentID(ctx, content.ReplyMessageID),
	}
	if len(content.At) > 0 {
		request.Content.(map[string]any)["at"] = content.At
	}

	result, err := d.API.SendMessage(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("发送消息: %w", err)
	}

	messageID := int64(0)
	if msgID := result.ID(); msgID != "" {
		messageID, err = d.Messages.MessageID(ctx, msgID)
		if err != nil {
			return nil, fmt.Errorf("记录发送消息的标识: %w", err)
		}
	} else {
		d.logger().Warn("云湖没有返回消息标识，引用与撤回将不可用", "recv_type", recvType, "recv_id", recvID)
	}

	return map[string]any{"message_id": messageID}, nil
}

// parentID 把引用的整数标识反查为云湖消息标识，查不到时返回空。
func (d *Dispatcher) parentID(ctx context.Context, messageID int64) string {
	if messageID == 0 {
		return ""
	}
	if d.Messages == nil {
		return ""
	}

	msgID, ok, err := d.Messages.MsgID(ctx, messageID)
	if err != nil {
		d.logger().Warn("反查引用消息失败，已忽略引用", "message_id", messageID, "err", err)
		return ""
	}
	if !ok {
		d.logger().Warn("引用的消息不在映射表中，已忽略引用", "message_id", messageID)
		return ""
	}

	return msgID
}

// badRequest 把参数解析失败包装成 100。
func badRequest(action string, err error) *obmodel.ActionError {
	return &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: action + " 的参数无法解析: " + err.Error()}
}

// logger 返回可用的日志实例。
func (d *Dispatcher) logger() *slog.Logger {
	if d.Logger == nil {
		return logging.Discard()
	}
	return d.Logger
}
