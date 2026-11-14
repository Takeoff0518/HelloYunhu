// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cqcode"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// messageParams 是消息类接口的参数。
type messageParams struct {
	MessageID int64 `json:"message_id"`
}

// getMessage 处理 get_msg，从最近的缓存中回放完整事件。
//
// 云湖没有按消息标识查询完整事件的接口，只能回放进程内缓存。缓存未命中时如实
// 返回 1404，不伪造消息内容。
func (d *Dispatcher) getMessage(_ context.Context, params json.RawMessage) (any, error) {
	var value messageParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("get_msg", err)
	}
	if value.MessageID <= 0 {
		return nil, &obmodel.ActionError{Retcode: obmodel.RetcodeBadRequest, Wording: "get_msg 缺少 message_id"}
	}

	if d.Recent == nil {
		return nil, messageNotCached(value.MessageID)
	}

	event, ok := d.Recent.Get(value.MessageID)
	if !ok {
		return nil, messageNotCached(value.MessageID)
	}

	return event, nil
}

// parseSendMessage 解析发送接口的 message 参数。
//
// 框架可能给出消息段数组，也可能给出 CQ 码字符串，两种形态都接受。解析失败时
// 返回错误，由调用方如实回给框架。
func parseSendMessage(raw json.RawMessage) (obmodel.InMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("缺少 message")
	}

	var segments obmodel.InMessage
	if err := json.Unmarshal(raw, &segments); err == nil {
		return segments, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, errors.New("message 既不是消息段数组，也不是字符串")
	}

	return fromCQCode(text), nil
}

// fromCQCode 把 CQ 码字符串转换为消息段。
func fromCQCode(text string) obmodel.InMessage {
	parsed := cqcode.Parse(text)

	segments := make(obmodel.InMessage, 0, len(parsed))
	for _, segment := range parsed {
		data, err := json.Marshal(segment.Data)
		if err != nil {
			continue
		}
		segments = append(segments, obmodel.InSegment{Type: segment.Type, Data: data})
	}

	return segments
}

// messageNotCached 返回消息不在缓存中的错误。
func messageNotCached(messageID int64) *obmodel.ActionError {
	return &obmodel.ActionError{
		Retcode: obmodel.RetcodeNotFound,
		Wording: fmt.Sprintf("消息 %d 不在最近的消息缓存中，可能已过期或进程已重启", messageID),
	}
}
