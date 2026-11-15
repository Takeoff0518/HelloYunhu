// SPDX-License-Identifier: GPL-3.0-or-later

// Package event 把云湖事件转换为 OneBot 事件。
//
// 转换只做语义映射，不负责发送与重连。所有标识在这里完成换算：云湖的字符串标识
// 变成 OneBot 的整数标识，原始标识保存在扩展字段中，便于排查。
package event

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cache"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
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
	// Media 下载媒体内容，为空时图片退化为直接给出地址。
	Media *media.Loader
	// Nicknames 记录见过的昵称，发送 @ 时用来还原正文里要写的名字。
	Nicknames *cache.Nicknames
	// Origins 记录消息属于哪个会话，撤回时使用。
	Origins *cache.Origins
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
	// 发送者的昵称是现成的，留着给发送 @ 用。
	c.Nicknames.Set(payload.Sender.SenderID, payload.Sender.SenderNickname)

	messageID, err := c.Messages.MessageID(ctx, payload.Message.MsgID)
	if err != nil {
		return nil, false, fmt.Errorf("分配消息标识: %w", err)
	}
	// 记下会话归属，框架之后要求撤回这条消息时需要。
	c.Origins.Put(messageID, payload.Chat.ChatID, payload.Chat.ChatType)

	// 指令消息的正文只有参数，指令名单独放在 message 里。OneBot 没有原生命令，
	// 框架按消息正文匹配命令，因此把指令名拼回正文，形如「/quote test」。
	command := instructionCommand(envelope.Header.EventType, &payload.Message)

	segments, rawMessage := c.segments(ctx, &payload.Message, command)

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
			Role:     role(payload.Sender.SenderUserLevel),
		},
		Yunhu: &obmodel.YunhuExtension{
			MsgID:       payload.Message.MsgID,
			ChatID:      payload.Chat.ChatID,
			ChatType:    payload.Chat.ChatType,
			EventID:     envelope.Header.EventID,
			Instruction: command,
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
//
// command 非空时拼在文本正文之前，与 OneBot 框架按正文匹配命令的习惯一致。
func (c *Converter) segments(ctx context.Context, payload *model.Message, command string) (obmodel.OutMessage, string) {
	switch payload.ContentType {
	case model.ContentTypeText:
		content, err := payload.TextContent()
		if err != nil {
			c.logger().Warn("解析文本内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
			return message.IncomingPlaceholder(payload.ContentType), ""
		}
		// 别人 @ 别人时正文里带着昵称，记下来供发送方向使用。
		for _, pair := range message.MentionNames(content) {
			c.Nicknames.Set(pair[0], pair[1])
		}
		if command != "" {
			content.Text = joinCommand(command, content.Text)
		}
		return message.IncomingText(content), content.Text

	case model.ContentTypeImage:
		return c.imageSegments(ctx, payload)

	case model.ContentTypeFile:
		content, err := payload.FileContent()
		if err != nil {
			c.logger().Warn("解析文件内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
			return message.IncomingPlaceholder(payload.ContentType), ""
		}
		file := c.mediaSource(ctx, "文件", content.FileURL)
		return obmodel.OutMessage{message.IncomingFile(file, content.FileURL, content.FileName)}, message.PlaceholderFile

	case model.ContentTypeVideo:
		content, err := payload.VideoContent()
		if err != nil {
			c.logger().Warn("解析视频内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
			return message.IncomingPlaceholder(payload.ContentType), ""
		}
		file := c.mediaSource(ctx, "视频", content.VideoURL)
		return obmodel.OutMessage{message.IncomingVideo(file, content.VideoURL)}, message.PlaceholderVideo

	case model.ContentTypeAudio:
		content, err := payload.AudioContent()
		if err != nil {
			c.logger().Warn("解析语音内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
			return message.IncomingPlaceholder(payload.ContentType), ""
		}
		file := c.mediaSource(ctx, "语音", content.AudioURL)
		return obmodel.OutMessage{message.IncomingAudio(file, content.AudioURL)}, message.PlaceholderAudio

	default:
		return message.IncomingPlaceholder(payload.ContentType), ""
	}
}

// imageSegments 把图片内容转换为消息段。
//
// 云湖的图片是网络地址，而框架访问云湖 CDN 时可能受到来源限制，因此这里先下载
// 再以 base64 交给框架；下载失败时退化为直接给出地址，至少不会丢消息。
func (c *Converter) imageSegments(ctx context.Context, payload *model.Message) (obmodel.OutMessage, string) {
	content, err := payload.ImageContent()
	if err != nil {
		c.logger().Warn("解析图片内容失败，已降级为占位文本", "msg_id", payload.MsgID, "err", err)
		return message.IncomingPlaceholder(payload.ContentType), ""
	}

	if c.Media != nil {
		name, data, err := c.Media.Load(ctx, content.ImageURL)
		if err == nil {
			file := "base64://" + base64.StdEncoding.EncodeToString(data)
			return obmodel.OutMessage{message.IncomingImage(file, content.ImageURL, name)}, message.PlaceholderImage
		}
		c.logger().Warn("下载图片失败，已退化为直接给出地址", "msg_id", payload.MsgID, "err", err)
	}

	name := content.ImageName
	segment := message.IncomingImage(content.ImageURL, content.ImageURL, name)
	return obmodel.OutMessage{segment}, message.PlaceholderImage
}

// mediaSource 把云湖的文件、视频与语音下载成 base64。
//
// 这些媒体同样受 CDN 的来源限制，框架直接拿地址去下载会得到 403，所以先在本进程
// 里下载再交给框架。下载失败时退回原地址，至少留下可追溯的线索。
func (c *Converter) mediaSource(ctx context.Context, kind, rawURL string) string {
	if c.Media == nil || rawURL == "" {
		return rawURL
	}

	_, data, err := c.Media.Load(ctx, rawURL)
	if err != nil {
		c.logger().Warn("下载媒体失败，已退化为直接给出地址", "kind", kind, "err", err)
		return rawURL
	}

	return "base64://" + base64.StdEncoding.EncodeToString(data)
}

// instructionCommand 返回指令消息的指令名，其它事件返回空。
//
// 云湖的指令消息把指令名放在 instructionName 或 commandName，正文只含参数。
func instructionCommand(eventType string, payload *model.Message) string {
	if eventType != model.EventTypeMessageInstruction {
		return ""
	}

	for _, candidate := range []string{payload.InstructionName, payload.CommandName} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// joinCommand 把指令名与正文拼成一条完整消息，正文为空时只返回指令名。
func joinCommand(command, text string) string {
	text = strings.TrimLeft(text, " ")
	if text == "" {
		return command
	}
	return command + " " + text
}

// role 把云湖的成员角色转换为 OneBot 的角色取值。
//
// 实测 senderUserLevel 取 owner 这类字符串；未知值时按普通成员处理，避免把不认识
// 的角色写成空值。
func role(level string) string {
	switch level {
	case "owner", "admin", "member":
		return level
	default:
		return "member"
	}
}

// logger 返回可用的日志实例。
func (c *Converter) logger() *slog.Logger {
	if c.Logger == nil {
		return logging.Discard()
	}
	return c.Logger
}
