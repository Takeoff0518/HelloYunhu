// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "encoding/json"

// BotInfo 是 GET /bot/info 返回的机器人信息。
type BotInfo struct {
	// BotID 是机器人标识，用于 OneBot 的 self_id 与订阅连接的鉴权。
	BotID string `json:"botId"`
	// Nickname 是机器人昵称。
	Nickname string `json:"nickname"`
	// AvatarURL 是机器人头像地址，兼容模式的头像代理会用到。
	AvatarURL string `json:"avatarUrl"`
}

// SendMessageRequest 是 POST /bot/send 的请求体。
type SendMessageRequest struct {
	// RecvID 是接收方标识：群聊为群标识，私聊为用户标识。
	RecvID string `json:"recvId"`
	// RecvType 取 group 或 user。
	RecvType string `json:"recvType"`
	// ContentType 是内容类型，一条消息只能有一个。
	ContentType string `json:"contentType"`
	// Content 的结构由 ContentType 决定，文本消息是 TextContent。
	Content any `json:"content"`
	// ParentID 是引用回复的目标消息标识，没有引用时留空。
	ParentID string `json:"parentId,omitempty"`
}

// SendMessageResult 是发送消息返回的数据。
// 平台实际使用的写法是 messageInfo.msgId，实测于 POST /bot/send；早期调研里猜测
// 的顶层 msgId 与 messageId 也一并兼容，调用方应使用 ID 方法取值。
type SendMessageResult struct {
	MsgID     string `json:"msgId"`
	MessageID string `json:"messageId"`
	SendTime  int64  `json:"sendTime"`
	// MessageInfo 是实测的返回结构，消息标识与接收方信息都在里面。
	MessageInfo MessageInfo `json:"messageInfo"`
}

// MessageInfo 是发送成功后的消息信息。
type MessageInfo struct {
	MsgID    string `json:"msgId"`
	RecvID   string `json:"recvId"`
	RecvType string `json:"recvType"`
}

// ID 返回发送成功后的消息标识，按实测结构优先取 messageInfo.msgId。
func (r *SendMessageResult) ID() string {
	switch {
	case r.MessageInfo.MsgID != "":
		return r.MessageInfo.MsgID
	case r.MsgID != "":
		return r.MsgID
	default:
		return r.MessageID
	}
}

// UnmarshalJSON 让 data 为字符串时也能解析出消息标识。
//
// 发送接口可能直接返回消息标识字符串而不是对象，这里两种形态都接受。
func (r *SendMessageResult) UnmarshalJSON(raw []byte) error {
	type alias SendMessageResult
	var value alias
	if err := json.Unmarshal(raw, &value); err == nil {
		*r = SendMessageResult(value)
		return nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	r.MsgID = text
	return nil
}
