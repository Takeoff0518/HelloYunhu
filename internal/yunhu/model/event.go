// SPDX-License-Identifier: GPL-3.0-or-later

// Package model 定义云湖开放平台的事件与接口数据结构。
//
// 字段名与结构与调研报告第 2 节一致。云湖的标识全部是字符串，消息标识是 32 位
// 十六进制，切换到 OneBot 侧之前必须经过标识映射。
package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

// 事件类型。
const (
	EventTypeMessageNormal      = "message.receive.normal"
	EventTypeMessageInstruction = "message.receive.instruction"
	EventTypeBotFollowed        = "bot.followed"
	EventTypeBotUnfollowed      = "bot.unfollowed"
	EventTypeGroupJoin          = "group.join"
	EventTypeGroupLeave         = "group.leave"
)

// 消息内容类型。一条消息只有一个内容类型。
const (
	ContentTypeText     = "text"
	ContentTypeImage    = "image"
	ContentTypeFile     = "file"
	ContentTypeVideo    = "video"
	ContentTypeAudio    = "audio"
	ContentTypeMarkdown = "markdown"
	ContentTypeHTML     = "html"
)

// 聊天类型。注意撤回接口使用 user 与 group，与事件中的 bot 与 group 不同。
const (
	ChatTypeGroup = "group"
	ChatTypeBot   = "bot"
)

// 接收方类型，用于发送消息接口。
const (
	RecvTypeUser  = "user"
	RecvTypeGroup = "group"
)

// Envelope 是订阅连接推送的一条完整事件。
type Envelope struct {
	Version string          `json:"version"`
	Header  Header          `json:"header"`
	Event   json.RawMessage `json:"event"`
}

// Header 是事件的公共头部。
type Header struct {
	// EventID 是事件的唯一标识，用于去重。
	EventID string `json:"eventId"`
	// EventTime 是平台产生事件的时间，单位毫秒。
	EventTime int64 `json:"eventTime"`
	// EventType 决定 Event 部分按哪个结构解析。
	EventType string `json:"eventType"`
}

// MessageEvent 是 message.receive.normal 与 message.receive.instruction 的负载。
type MessageEvent struct {
	Sender  Sender  `json:"sender"`
	Chat    Chat    `json:"chat"`
	Message Message `json:"message"`
}

// Sender 是消息发送者。
//
// SenderUserLevel 实测是字符串，取值为 owner、admin、member 这类角色，而不是等级
// 数字。早期调研把它记成数字，实际事件里是 "owner"。
type Sender struct {
	SenderID        string `json:"senderId"`
	SenderType      string `json:"senderType"`
	SenderNickname  string `json:"senderNickname"`
	SenderUserLevel string `json:"senderUserLevel"`
	SenderAvatarURL string `json:"senderAvatarUrl"`
}

// Chat 是消息所在的会话。
type Chat struct {
	// ChatID 在群聊中是群标识，在私聊中是机器人标识。
	ChatID string `json:"chatId"`
	// ChatType 取 group 或 bot。
	ChatType string `json:"chatType"`
}

// Message 是消息本体。
type Message struct {
	MsgID       string `json:"msgId"`
	ParentID    string `json:"parentId"`
	SendTime    int64  `json:"sendTime"`
	ChatID      string `json:"chatId"`
	ChatType    string `json:"chatType"`
	ContentType string `json:"contentType"`
	// Content 的结构由 ContentType 决定，需要按类型解析。
	Content json.RawMessage `json:"content"`

	// 下面四个字段只在指令消息中出现，实测位于 message 内，标识为数字。
	InstructionID   int64  `json:"instructionId"`
	InstructionName string `json:"instructionName"`
	CommandID       int64  `json:"commandId"`
	CommandName     string `json:"commandName"`
}

// TextContent 是 contentType 为 text 时的消息内容。
type TextContent struct {
	// Text 是消息正文。被 @ 的人在正文中以 @昵称 的形式出现。
	Text string `json:"text"`
	// At 是被 @ 的用户标识列表，正文里没有对应关系，需要按昵称切分。
	At []string `json:"at"`
	// Buttons 是按钮布局，首版不处理，保留原始内容。
	Buttons json.RawMessage `json:"buttons"`
}

// ErrUnsupportedContent 表示消息的内容类型还没有实现转换。
var ErrUnsupportedContent = errors.New("暂不支持的消息内容类型")

// TextContent 解析文本内容。内容类型不是 text 时返回 ErrUnsupportedContent。
func (m *Message) TextContent() (*TextContent, error) {
	if m.ContentType != ContentTypeText {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedContent, m.ContentType)
	}

	var content TextContent
	if err := json.Unmarshal(m.Content, &content); err != nil {
		return nil, fmt.Errorf("解析文本内容: %w", err)
	}

	return &content, nil
}
