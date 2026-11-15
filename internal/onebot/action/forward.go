// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"strconv"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// forwardParams 是合并转发类接口的参数。
//
// messages 与发送接口的 message 一样，是节点消息段数组，元素类型为 node。
type forwardParams struct {
	MessageType string          `json:"message_type"`
	GroupID     int64           `json:"group_id"`
	UserID      int64           `json:"user_id"`
	Messages    json.RawMessage `json:"messages"`
}

// sendForward 把一组转发节点展开成文本后发送。
//
// 云湖没有合并转发类型，这里按节点逐行展开成文本，超过单段上限时拆成多条消息。
// recvType 为空时按参数里的标识推断接收方。
func (d *Dispatcher) sendForward(ctx context.Context, name string, params json.RawMessage, recvType string) (any, error) {
	var value forwardParams
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest(name, err)
	}

	if recvType == "" {
		switch {
		case value.GroupID > 0:
			recvType = yunhumodel.RecvTypeGroup
		case value.UserID > 0:
			recvType = yunhumodel.RecvTypeUser
		default:
			return nil, &obmodel.ActionError{
				Retcode: obmodel.RetcodeBadRequest,
				Wording: name + " 缺少 group_id 或 user_id",
			}
		}
	}

	var recvID string
	if recvType == yunhumodel.RecvTypeGroup {
		if value.GroupID <= 0 {
			return nil, &obmodel.ActionError{
				Retcode: obmodel.RetcodeBadRequest,
				Wording: name + " 缺少 group_id",
			}
		}
		recvID = strconv.FormatInt(value.GroupID, 10)
	} else {
		if value.UserID <= 0 {
			return nil, &obmodel.ActionError{
				Retcode: obmodel.RetcodeBadRequest,
				Wording: name + " 缺少 user_id",
			}
		}
		recvID = strconv.FormatInt(value.UserID, 10)
	}

	segments, err := parseForwardMessages(name, value.Messages)
	if err != nil {
		return nil, err
	}

	return d.send(ctx, recvType, recvID, segments)
}

// parseForwardMessages 解析 messages 参数。
func parseForwardMessages(name string, raw json.RawMessage) (obmodel.InMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: name + " 缺少 messages",
		}
	}

	var segments obmodel.InMessage
	if err := json.Unmarshal(raw, &segments); err != nil {
		return nil, badRequest(name, err)
	}
	if len(segments) == 0 {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: name + " 的 messages 不能为空",
		}
	}

	for index, segment := range segments {
		if segment.Type != "node" && segment.Type != "forward" {
			return nil, &obmodel.ActionError{
				Retcode: obmodel.RetcodeBadRequest,
				Wording: name + " 的 messages 第 " + strconv.Itoa(index+1) + " 项不是 node 段",
			}
		}
	}

	return segments, nil
}
