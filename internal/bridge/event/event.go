// SPDX-License-Identifier: GPL-3.0-or-later

// Package event 把云湖事件转换为 OneBot 事件。
//
// 转换只做语义映射，不负责发送与重连。所有标识在这里完成换算：云湖的字符串标识
// 变成 OneBot 的整数标识，原始标识保存在扩展字段中，便于排查。
package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/message"
	"github.com/Takeoff0518/HelloYunhu/internal/logging"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// Converter 把云湖事件转换为 OneBot 事件。
type Converter struct {
	// SelfID 是机器人自身的整数标识，用于 self_id 字段与自身消息过滤。
	SelfID int64
	// BotID 是机器人自身的云湖标识，用于过滤机器人自己发出的消息。
	BotID string
	// Messages 是消息标识映射。
	Messages identity.MessageStore
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
}

// Convert 转换一条事件。
//
// 第二个返回值表示这条事件是否应当发给框架：机器人自己的消息与尚未实现的事件
// 类型会被跳过，此时返回 false，且不返回错误。真正的解析失败返回错误，由调用方
// 记录日志。
func (c *Converter) Convert(ctx context.Context, envelope model.Envelope) (any, bool, error) {
	switch envelope.Header.EventType {
	case model.EventTypeMessageNormal, model.EventTypeMessageInstruction:
		return c.messageEvent(ctx, envelope)
	case model.EventTypeBotFollowed, model.EventTypeBotUnfollowed,
		model.EventTypeGroupJoin, model.EventTypeGroupLeave:
		// 通知类事件在 M3 接入，先明确记录，避免看起来像丢弃。
		c.logger().Info("通知类事件尚未实现，已跳过", "event_type", envelope.Header.EventType)
		return nil, false, nil
	default:
		c.logger().Debug("未知事件类型，已跳过", "event_type", envelope.Header.EventType)
		return nil, false, nil
	}
}

// Lifecycle 构造连接建立事件。框架据此打印「已连接」。
func Lifecycle(selfID int64, now time.Time) obmodel.LifecycleEvent {
	return obmodel.LifecycleEvent{
		PostType:      obmodel.PostTypeMetaEvent,
		MetaEventType: obmodel.MetaEventTypeLifecycle,
		SubType:       "connect",
		Time:          now.Unix(),
		SelfID:        selfID,
	}
}

// messageEvent 转换消息事件。
func (c *Converter) messageEvent(ctx context.Context, envelope model.Envelope) (any, bool, error) {
	var payload model.MessageEvent
	if err := json.Unmarshal(envelope.Event, &payload); err != nil {
		return nil, false, fmt.Errorf("解析消息事件: %w", err)
	}

	// 云湖是否推送机器人自己的消息尚未实测，这里先按标识过滤，避免自问自答。
	if c.BotID != "" && payload.Sender.SenderID == c.BotID {
		c.logger().Debug("忽略机器人自己发出的消息", "msg_id", payload.Message.MsgID)
		return nil, false, nil
	}

	userID, exact := identity.ParseID(payload.Sender.SenderID)
	if !exact {
		c.logger().Warn("发送者标识无法解析为整数，已退化为哈希值", "sender_id", payload.Sender.SenderID)
	}

	messageID, err := c.Messages.MessageID(ctx, payload.Message.MsgID)
	if err != nil {
		return nil, false, fmt.Errorf("分配消息标识: %w", err)
	}

	segments, rawMessage := c.segments(&payload.Message)

	event := &obmodel.MessageEvent{
		PostType:   obmodel.PostTypeMessage,
		SubType:    "normal",
		Time:       envelope.Header.EventTime / 1000,
		SelfID:     c.SelfID,
		MessageID:  messageID,
		UserID:     userID,
		Message:    segments,
		RawMessage: rawMessage,
		Font:       0,
		Sender: obmodel.Sender{
			UserID:   userID,
			Nickname: payload.Sender.SenderNickname,
			Role:     "member",
		},
		Yunhu: &obmodel.YunhuExtension{
			MsgID:    payload.Message.MsgID,
			ChatID:   payload.Chat.ChatID,
			ChatType: payload.Chat.ChatType,
			EventID:  envelope.Header.EventID,
		},
	}

	switch payload.Chat.ChatType {
	case model.ChatTypeGroup:
		groupID, groupExact := identity.ParseID(payload.Chat.ChatID)
		if !groupExact {
			c.logger().Warn("群标识无法解析为整数，已退化为哈希值", "chat_id", payload.Chat.ChatID)
		}
		event.MessageType = obmodel.MessageTypeGroup
		event.GroupID = groupID
	case model.ChatTypeBot:
		// 私聊事件里的 chatId 是机器人标识，用户标识在 sender 里。
		event.MessageType = obmodel.MessageTypePrivate
		event.SubType = "friend"
	default:
		c.logger().Info("未知会话类型，已跳过", "chat_type", payload.Chat.ChatType)
		return nil, false, nil
	}

	if event.Time == 0 {
		event.Time = time.Now().Unix()
	}

	return event, true, nil
}

// segments 把消息内容转换为消息段与原始文本。
func (c *Converter) segments(payload *model.Message) (obmodel.OutMessage, string) {
	if payload.ContentType == model.ContentTypeText {
		content, err := payload.TextContent()
		if err != nil {
			c.logger().Warn("解析文本内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
			return message.IncomingPlaceholder(payload.ContentType), ""
		}
		return message.IncomingText(content), content.Text
	}

	return message.IncomingPlaceholder(payload.ContentType), ""
}

// logger 返回可用的日志实例。
func (c *Converter) logger() *slog.Logger {
	if c.Logger == nil {
		return logging.Discard()
	}
	return c.Logger
}
