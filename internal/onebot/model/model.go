// SPDX-License-Identifier: GPL-3.0-or-later

// Package model 定义 OneBot v11 的事件、消息段与接口结构。
//
// 结构按 aiocqhttp 的实际要求组织：事件必须带 post_type 与对应的子类型，
// 消息的 message 字段必须是数组，sender 必须存在。
package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// 状态字段取值。
const (
	StatusOK     = "ok"
	StatusFailed = "failed"
)

// 返回码。框架对返回码的检查并不严格，但规范填写便于排查问题。
const (
	// RetcodeOK 表示调用成功。
	RetcodeOK = 0
	// RetcodeBadRequest 表示参数不合法。
	RetcodeBadRequest = 100
	// RetcodeNotFound 表示接口未实现或目标不存在。
	RetcodeNotFound = 1404
	// RetcodeServerError 表示内部错误，例如上游接口失败。
	RetcodeServerError = 1500
)

// 事件类型。
const (
	PostTypeMessage   = "message"
	PostTypeMetaEvent = "meta_event"
	PostTypeNotice    = "notice"
)

// 通知类型。好友相关的两个类型不属于 OneBot 标准，取值与调研报告 6.1 节一致。
const (
	NoticeTypeFriendAdd     = "friend_add"
	NoticeTypeFriendRemove  = "yunhu_friend_remove"
	NoticeTypeGroupIncrease = "group_increase"
	NoticeTypeGroupDecrease = "group_decrease"
	SubTypeApprove          = "approve"
	SubTypeLeave            = "leave"
)

// 消息类型。
const (
	MessageTypeGroup   = "group"
	MessageTypePrivate = "private"
)

// 元事件类型。
const (
	MetaEventTypeLifecycle = "lifecycle"
	MetaEventTypeHeartbeat = "heartbeat"
)

// Request 是框架发来的接口调用。
type Request struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   json.RawMessage `json:"echo"`
}

// Response 是发给框架的接口响应。
//
// status 与 echo 是硬性要求：缺少 status 会让框架抛异常，echo 没有原样返回会
// 让每次调用等到超时。
type Response struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    any             `json:"data"`
	Echo    json.RawMessage `json:"echo,omitempty"`
	Wording string          `json:"wording,omitempty"`
}

// Success 构造成功响应。
func Success(data any, echo json.RawMessage) Response {
	return Response{Status: StatusOK, Retcode: RetcodeOK, Data: data, Echo: echo}
}

// Failure 构造失败响应。
func Failure(retcode int, wording string, echo json.RawMessage) Response {
	return Response{Status: StatusFailed, Retcode: retcode, Data: nil, Echo: echo, Wording: wording}
}

// ActionError 描述一次接口调用的失败，携带 OneBot 返回码与说明。
type ActionError struct {
	// Retcode 是返回给框架的返回码，例如未实现的接口使用 1404。
	Retcode int
	// Wording 是给人看的说明，会写进响应的 wording 字段。
	Wording string
}

// Error 实现 error 接口。
func (e *ActionError) Error() string {
	return e.Wording
}

// NotImplemented 返回未实现接口的标准错误。
func NotImplemented(action string) *ActionError {
	return &ActionError{Retcode: RetcodeNotFound, Wording: "接口 " + action + " 尚未实现"}
}

// OutSegment 是发往框架的消息段，Data 是具体的内容结构。
type OutSegment struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// OutMessage 是发往框架的消息段数组。框架要求 message 必须是数组。
type OutMessage []OutSegment

// 消息段数据。
type (
	// TextData 是文本段。
	TextData struct {
		Text string `json:"text"`
	}
	// AtData 是 @ 段。
	AtData struct {
		QQ int64 `json:"qq"`
	}
	// ReplyData 是引用段。ID 兼容数字与字符串两种写法。
	ReplyData struct {
		ID IDValue `json:"id"`
	}
	// ImageData 是图片段。File 可以是 base64://、网络地址或本地路径。
	ImageData struct {
		File string `json:"file"`
		URL  string `json:"url,omitempty"`
		Name string `json:"name,omitempty"`
	}
	// FileData 是文件段。
	FileData struct {
		File string `json:"file"`
		URL  string `json:"url,omitempty"`
		Name string `json:"name,omitempty"`
	}
	// VideoData 是视频段。
	VideoData struct {
		File string `json:"file"`
		URL  string `json:"url,omitempty"`
	}
	// RecordData 是语音段。云湖没有语音发送能力，接收方向转成文件。
	RecordData struct {
		File string `json:"file"`
		URL  string `json:"url,omitempty"`
	}
)

// Text 构造文本段。
func Text(text string) OutSegment {
	return OutSegment{Type: "text", Data: TextData{Text: text}}
}

// At 构造 @ 段。
func At(qq int64) OutSegment {
	return OutSegment{Type: "at", Data: AtData{QQ: qq}}
}

// Reply 构造引用段，id 是被引用消息的整数标识。
func Reply(id int64) OutSegment {
	return OutSegment{Type: "reply", Data: ReplyData{ID: IDValue(id)}}
}

// InSegment 是框架发来的消息段，Data 保持原始内容，由具体接口解析。
type InSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// InMessage 是框架发来的消息段数组。
type InMessage []InSegment

// Sender 是消息发送者。框架要求 sender 必须存在。
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card,omitempty"`
	Role     string `json:"role,omitempty"`
}

// YunhuExtension 是附加在事件上的云湖原始信息。
//
// 标识映射会让框架侧看到的是整数，这里保留云湖的字符串标识，便于插件直接使用，
// 也便于排查映射问题。
type YunhuExtension struct {
	MsgID    string `json:"msg_id"`
	ChatID   string `json:"chat_id"`
	ChatType string `json:"chat_type"`
	EventID  string `json:"event_id,omitempty"`
	// Instruction 是指令消息的指令名，普通消息为空。
	//
	// 指令名已经拼进正文，便于框架按正文匹配命令；这个字段留给需要精确判断的插件。
	Instruction string `json:"instruction,omitempty"`
}

// NoticeExtension 是附加在通知事件上的云湖原始信息。
//
// 通知事件没有消息标识，只保留事件标识与原始字符串标识，便于插件与排查使用。
type NoticeExtension struct {
	EventID  string `json:"event_id,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

// NoticeEvent 是通知类事件，覆盖好友关注、取关与群成员增减。
//
// OneBot 的通知事件字段比较松散，这里把所有可能出现的字段都声明出来，空字段
// 用 omitempty 省略；框架按 post_type 与 notice_type 分派，不会因为多余字段报错。
type NoticeEvent struct {
	PostType   string           `json:"post_type"`
	NoticeType string           `json:"notice_type"`
	SubType    string           `json:"sub_type,omitempty"`
	Time       int64            `json:"time"`
	SelfID     int64            `json:"self_id"`
	UserID     int64            `json:"user_id"`
	GroupID    int64            `json:"group_id,omitempty"`
	OperatorID int64            `json:"operator_id,omitempty"`
	Yunhu      *NoticeExtension `json:"yunhu,omitempty"`
}

// MessageEvent 是消息事件。
type MessageEvent struct {
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	Time        int64           `json:"time"`
	SelfID      int64           `json:"self_id"`
	MessageID   int64           `json:"message_id"`
	UserID      int64           `json:"user_id"`
	GroupID     int64           `json:"group_id,omitempty"`
	Message     OutMessage      `json:"message"`
	RawMessage  string          `json:"raw_message"`
	Font        int             `json:"font"`
	Sender      Sender          `json:"sender"`
	Yunhu       *YunhuExtension `json:"yunhu,omitempty"`
}

// LifecycleEvent 是连接建立事件。框架据此打印「已连接」。
type LifecycleEvent struct {
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
	Time          int64  `json:"time"`
	SelfID        int64  `json:"self_id"`
}

// Status 是运行状态，出现在心跳事件中。
type Status struct {
	Online bool `json:"online"`
	Good   bool `json:"good"`
}

// HeartbeatEvent 是心跳事件。
type HeartbeatEvent struct {
	PostType      string `json:"post_type"`
	MetaEventType string `json:"meta_event_type"`
	Time          int64  `json:"time"`
	SelfID        int64  `json:"self_id"`
	Status        Status `json:"status"`
	Interval      int64  `json:"interval"`
}

// IDValue 是兼容数字与字符串的整数标识。
//
// 不同框架序列化标识的习惯不同：AstrBot 用数字，NoneBot2 的引用段用字符串。
// 解析两者都接受，避免因为一个字段的写法差异让整条调用失败。
type IDValue int64

// UnmarshalJSON 接受 JSON 数字或字符串形式的整数。
func (v *IDValue) UnmarshalJSON(raw []byte) error {
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		*v = IDValue(number)
		return nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}

	parsed, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return fmt.Errorf("标识 %q 不是整数: %w", text, err)
	}

	*v = IDValue(parsed)
	return nil
}

// Int64 返回标准整数。
func (v IDValue) Int64() int64 {
	return int64(v)
}
