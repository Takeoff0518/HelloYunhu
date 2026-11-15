// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// noticeEvent 转换通知类事件。
//
// 云湖推送的关注、取关、入群与退群都是扁平结构，没有操作者字段，operator_id 统一
// 留 0。云湖没有撤回事件，所以这里不处理撤回。
func (c *Converter) noticeEvent(envelope model.Envelope) (any, bool, error) {
	switch envelope.Header.EventType {
	case model.EventTypeBotFollowed, model.EventTypeBotUnfollowed:
		return c.followEvent(envelope)
	case model.EventTypeGroupJoin, model.EventTypeGroupLeave:
		return c.groupNoticeEvent(envelope)
	default:
		c.logger().Debug("未知通知事件类型，已跳过", "event_type", envelope.Header.EventType)
		return nil, false, nil
	}
}

// followEvent 转换关注与取关事件。
func (c *Converter) followEvent(envelope model.Envelope) (any, bool, error) {
	var payload model.FollowEvent
	if err := json.Unmarshal(envelope.Event, &payload); err != nil {
		return nil, false, fmt.Errorf("解析关注事件: %w", err)
	}
	if payload.UserID == "" {
		// 没有用户标识的通知没法交给框架，跳过时留下线索。
		c.logger().Warn("关注事件缺少用户标识，已跳过", "event_type", envelope.Header.EventType)
		return nil, false, nil
	}

	userID, exact := identity.ParseID(payload.UserID)
	if !exact {
		c.logger().Warn("关注事件里的用户标识无法解析为整数，已退化为哈希值", "user_id", payload.UserID)
	}

	noticeType := obmodel.NoticeTypeFriendAdd
	if envelope.Header.EventType == model.EventTypeBotUnfollowed {
		noticeType = obmodel.NoticeTypeFriendRemove
	}

	return &obmodel.NoticeEvent{
		PostType:   obmodel.PostTypeNotice,
		NoticeType: noticeType,
		Time:       noticeTime(envelope, payload.Time),
		SelfID:     c.SelfID,
		UserID:     userID,
		Yunhu: &obmodel.NoticeExtension{
			EventID:  envelope.Header.EventID,
			UserID:   payload.UserID,
			Nickname: payload.Nickname,
		},
	}, true, nil
}

// groupNoticeEvent 转换入群与退群事件。
func (c *Converter) groupNoticeEvent(envelope model.Envelope) (any, bool, error) {
	var payload model.GroupNoticeEvent
	if err := json.Unmarshal(envelope.Event, &payload); err != nil {
		return nil, false, fmt.Errorf("解析群通知事件: %w", err)
	}
	if payload.GroupID == "" || payload.UserID == "" {
		c.logger().Warn("群通知事件缺少群标识或用户标识，已跳过",
			"event_type", envelope.Header.EventType,
			"group_id", payload.GroupID,
			"user_id", payload.UserID,
		)
		return nil, false, nil
	}

	groupID, groupExact := identity.ParseID(payload.GroupID)
	userID, userExact := identity.ParseID(payload.UserID)
	if !groupExact || !userExact {
		c.logger().Warn("群通知事件里的标识无法解析为整数，已退化为哈希值",
			"group_id", payload.GroupID,
			"user_id", payload.UserID,
		)
	}

	notice := &obmodel.NoticeEvent{
		PostType: obmodel.PostTypeNotice,
		Time:     noticeTime(envelope, payload.Time),
		SelfID:   c.SelfID,
		UserID:   userID,
		GroupID:  groupID,
		Yunhu: &obmodel.NoticeExtension{
			EventID:  envelope.Header.EventID,
			UserID:   payload.UserID,
			GroupID:  payload.GroupID,
			Nickname: payload.Nickname,
		},
	}

	switch envelope.Header.EventType {
	case model.EventTypeGroupJoin:
		notice.NoticeType = obmodel.NoticeTypeGroupIncrease
		notice.SubType = obmodel.SubTypeApprove
	case model.EventTypeGroupLeave:
		notice.NoticeType = obmodel.NoticeTypeGroupDecrease
		// 云湖的退群事件没有区分主动退出与被踢，统一按主动退出上报。
		notice.SubType = obmodel.SubTypeLeave
	}

	return notice, true, nil
}

// noticeTime 返回通知事件的时间，单位秒。
//
// 优先用事件头部的时间，缺失时退回负载里的时间；都没有就用当前时间，避免给出
// 1970 年这种明显错误的取值。
func noticeTime(envelope model.Envelope, payloadTime int64) int64 {
	if envelope.Header.EventTime > 0 {
		return envelope.Header.EventTime / 1000
	}
	if payloadTime > 0 {
		return payloadTime
	}
	return time.Now().Unix()
}
