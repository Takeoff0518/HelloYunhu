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
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cache"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
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
	// Media 读取待上传的媒体内容，为空时无法发送图片与文件。
	Media *media.Loader
	// Recent 保存最近的消息事件，供 get_msg 回放。
	Recent *cache.Messages
	// Nicknames 记录见过的昵称，发送 @ 时把正文里的标识换成昵称。
	Nicknames *cache.Nicknames
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
	case "get_group_info":
		return d.groupInfo(ctx, params)
	case "get_group_list":
		return d.groupList(ctx)
	case "get_group_member_info":
		return d.groupMemberInfo(ctx, params)
	case "get_group_member_list":
		return d.groupMemberList(ctx, params)
	case "get_stranger_info":
		return d.strangerInfo(ctx, params)
	case "get_friend_list":
		return d.friendList(), nil
	case "get_msg":
		return d.getMessage(ctx, params)
	case "get_status":
		return map[string]any{"online": true, "good": true}, nil
	case "delete_msg":
		return d.deleteMessage(ctx, params)
	case "set_group_ban":
		return d.setGroupBan(ctx, params)
	case "set_group_kick":
		return d.setGroupKick(ctx, params)
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
// message 既可能是消息段数组，也可能是 CQ 码字符串，解析前保持原始内容。
type sendParams struct {
	MessageType string          `json:"message_type"`
	GroupID     int64           `json:"group_id"`
	UserID      int64           `json:"user_id"`
	Message     json.RawMessage `json:"message"`
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

	segments, err := parseSendMessage(value.Message)
	if err != nil {
		return nil, badRequest("send_group_msg", err)
	}

	return d.send(ctx, yunhumodel.RecvTypeGroup, strconv.FormatInt(value.GroupID, 10), segments)
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

	segments, err := parseSendMessage(value.Message)
	if err != nil {
		return nil, badRequest("send_private_msg", err)
	}

	return d.send(ctx, yunhumodel.RecvTypeUser, strconv.FormatInt(value.UserID, 10), segments)
}

// sendMessage 处理 send_msg，按 message_type 决定接收方。
func (d *Dispatcher) sendMessage(ctx context.Context, params json.RawMessage) (any, error) {
	var value sendParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("send_msg", err)
	}

	segments, err := parseSendMessage(value.Message)
	if err != nil {
		return nil, badRequest("send_msg", err)
	}

	switch value.MessageType {
	case obmodel.MessageTypeGroup:
		if value.GroupID <= 0 {
			return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_msg 缺少 group_id"}
		}
		return d.send(ctx, yunhumodel.RecvTypeGroup, strconv.FormatInt(value.GroupID, 10), segments)
	case obmodel.MessageTypePrivate:
		if value.UserID <= 0 {
			return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "send_msg 缺少 user_id"}
		}
		return d.send(ctx, yunhumodel.RecvTypeUser, strconv.FormatInt(value.UserID, 10), segments)
	default:
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: fmt.Sprintf("send_msg 的 message_type %q 不受支持", value.MessageType),
		}
	}
}

// send 把消息段拆成若干条云湖消息依次发送。
//
// 文本、@ 与引用合并为一条文本消息，图片各自成为一条媒体消息。引用的目标只挂在
// 第一条消息上，避免一次发送多段时每段都带引用。返回第一条消息的整数标识。
func (d *Dispatcher) send(ctx context.Context, recvType, recvID string, segments obmodel.InMessage) (any, error) {
	if d.API == nil {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeServerError, Wording: "接口客户端尚未初始化"}
	}

	parts, err := message.Split(segments)
	if err != nil {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: err.Error()}
	}

	parentID := d.parentID(ctx, parts[0].ReplyMessageID)
	firstMessageID := int64(0)

	for index, part := range parts {
		request, err := d.buildRequest(ctx, recvType, recvID, part)
		if err != nil {
			return nil, err
		}
		if index == 0 {
			request.ParentID = parentID
		}

		// 只记录结构与标识，不输出消息正文。
		d.logger().Debug("发送消息",
			"recv_type", request.RecvType,
			"recv_id", request.RecvID,
			"content_type", request.ContentType,
			"parent_id", request.ParentID,
			"part_index", index,
		)

		result, err := d.API.SendMessage(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("发送消息: %w", err)
		}

		if firstMessageID != 0 {
			continue
		}

		msgID := result.ID()
		if msgID == "" {
			d.logger().Warn("云湖没有返回消息标识，引用与撤回将不可用", "recv_type", recvType, "recv_id", recvID)
			continue
		}

		firstMessageID, err = d.Messages.MessageID(ctx, msgID)
		if err != nil {
			return nil, fmt.Errorf("记录发送消息的标识: %w", err)
		}
		// 会话信息与标识一起落库，重启后仍可撤回。
		if err := d.Messages.SaveOrigin(ctx, firstMessageID, recvID, recvType); err != nil {
			d.logger().Warn("记录发送消息的归属失败，重启后可能无法撤回", "message_id", firstMessageID, "err", err)
		}
	}

	return map[string]any{"message_id": firstMessageID}, nil
}

// buildRequest 把一段内容转换成云湖的发送请求。
func (d *Dispatcher) buildRequest(ctx context.Context, recvType, recvID string, part message.Part) (yunhumodel.SendMessageRequest, error) {
	request := yunhumodel.SendMessageRequest{RecvID: recvID, RecvType: recvType}

	if part.Media != nil {
		contentType, content, err := d.uploadMedia(ctx, *part.Media)
		if err != nil {
			return yunhumodel.SendMessageRequest{}, err
		}
		request.ContentType = contentType
		request.Content = content
		return request, nil
	}

	content := map[string]any{"text": d.mentionText(part.Text, part.At)}
	if len(part.At) > 0 {
		content["at"] = part.At
	}
	request.ContentType = yunhumodel.ContentTypeText
	request.Content = content

	return request, nil
}

// mentionText 把正文里的 @标识 换成 @昵称。
//
// 云湖的正文需要昵称才会显示成提及，只给 at 数组时客户端显示的是标识。昵称来自
// 见过的消息事件，取不到时保持标识，至少还能看出 @ 的是谁。
func (d *Dispatcher) mentionText(text string, at []string) string {
	if d.Nicknames == nil {
		return text
	}

	for _, id := range at {
		nickname, ok := d.Nicknames.Get(id)
		if !ok {
			continue
		}
		text = strings.ReplaceAll(text, "@"+id, "@"+nickname)
	}

	return text
}

// uploadMedia 读取媒体内容并上传，返回云湖的内容类型与内容。
//
// 云湖没有语音发送能力，语音按文件发送，用户需要点开文件收听。
func (d *Dispatcher) uploadMedia(ctx context.Context, part message.MediaPart) (string, map[string]any, error) {
	if d.Media == nil {
		return "", nil, errors.New("媒体读取尚未初始化")
	}

	name, data, err := d.Media.Load(ctx, part.Source)
	if err != nil {
		return "", nil, fmt.Errorf("读取 %s: %w", part.Kind, err)
	}
	if part.Name != "" {
		name = part.Name
	}

	switch part.Kind {
	case "image":
		key, err := d.API.UploadImage(ctx, name, data)
		if err != nil {
			return "", nil, fmt.Errorf("上传图片: %w", err)
		}
		return yunhumodel.ContentTypeImage, map[string]any{"imageKey": key}, nil

	case "video":
		key, err := d.API.UploadVideo(ctx, name, data)
		if err != nil {
			return "", nil, fmt.Errorf("上传视频: %w", err)
		}
		return yunhumodel.ContentTypeVideo, map[string]any{"videoKey": key}, nil

	case "file", "record":
		key, err := d.API.UploadFile(ctx, name, data)
		if err != nil {
			return "", nil, fmt.Errorf("上传文件: %w", err)
		}
		return yunhumodel.ContentTypeFile, map[string]any{
			"fileName": name,
			"fileKey":  key,
			"fileSize": len(data),
		}, nil

	default:
		return "", nil, fmt.Errorf("不支持的媒体类型 %q", part.Kind)
	}
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
