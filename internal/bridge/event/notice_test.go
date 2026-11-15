// SPDX-License-Identifier: GPL-3.0-or-later

package event

import (
	"context"
	"testing"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// followedEvent 是关注事件的负载，字段按官方文档写。
const followedEvent = `{
  "version": "1.0",
  "header": {"eventId": "evt-follow", "eventTime": 1761936492340, "eventType": "bot.followed"},
  "event": {
    "userId": "2272345",
    "nickname": "洛雨辰",
    "avatarUrl": "https://example.com/a.png",
    "time": 1761936492340
  }
}`

// groupJoinEvent 是入群事件的负载。
const groupJoinEvent = `{
  "version": "1.0",
  "header": {"eventId": "evt-join", "eventTime": 1761936492340, "eventType": "group.join"},
  "event": {"chatId": "695367192", "userId": "10001", "nickname": "小明"}
}`

// convertNotice 转换一条通知事件并要求成功。
func convertNotice(t *testing.T, raw string) *obmodel.NoticeEvent {
	t.Helper()

	converted, ok, err := newConverter().Convert(context.Background(), parse(t, raw))
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if !ok {
		t.Fatal("期望发送通知事件")
	}

	notice, isNotice := converted.(*obmodel.NoticeEvent)
	if !isNotice {
		t.Fatalf("事件类型为 %T", converted)
	}
	return notice
}

func TestConvertFollowedNotice(t *testing.T) {
	notice := convertNotice(t, followedEvent)

	if notice.PostType != obmodel.PostTypeNotice || notice.NoticeType != obmodel.NoticeTypeFriendAdd {
		t.Errorf("通知类型为 %s/%s", notice.PostType, notice.NoticeType)
	}
	if notice.UserID != 2272345 || notice.SelfID != 1001 {
		t.Errorf("用户标识为 %d，自身标识为 %d", notice.UserID, notice.SelfID)
	}
	if notice.Time != 1761936492 {
		t.Errorf("时间为 %d", notice.Time)
	}
	if notice.Yunhu == nil || notice.Yunhu.EventID != "evt-follow" || notice.Yunhu.Nickname != "洛雨辰" {
		t.Errorf("扩展字段为 %+v", notice.Yunhu)
	}
}

func TestConvertUnfollowedNotice(t *testing.T) {
	raw := `{"header":{"eventId":"evt-unfollow","eventType":"bot.unfollowed"},"event":{"userId":"2272345"}}`
	notice := convertNotice(t, raw)

	if notice.NoticeType != obmodel.NoticeTypeFriendRemove {
		t.Errorf("通知类型为 %q", notice.NoticeType)
	}
}

func TestConvertGroupJoinNotice(t *testing.T) {
	notice := convertNotice(t, groupJoinEvent)

	if notice.NoticeType != obmodel.NoticeTypeGroupIncrease || notice.SubType != obmodel.SubTypeApprove {
		t.Errorf("通知类型为 %s/%s", notice.NoticeType, notice.SubType)
	}
	if notice.GroupID != 695367192 || notice.UserID != 10001 {
		t.Errorf("群标识为 %d，用户标识为 %d", notice.GroupID, notice.UserID)
	}
	if notice.OperatorID != 0 {
		t.Errorf("操作者标识应为 0，实际为 %d", notice.OperatorID)
	}
}

func TestConvertGroupLeaveNotice(t *testing.T) {
	raw := `{"header":{"eventType":"group.leave"},"event":{"chatId":"695367192","userId":"10001"}}`
	notice := convertNotice(t, raw)

	if notice.NoticeType != obmodel.NoticeTypeGroupDecrease || notice.SubType != obmodel.SubTypeLeave {
		t.Errorf("通知类型为 %s/%s", notice.NoticeType, notice.SubType)
	}
}

// TestNoticeToleratesFieldVariants 校验下划线字段与毫秒时间戳。
func TestNoticeToleratesFieldVariants(t *testing.T) {
	raw := `{"header":{"eventType":"bot.followed"},"event":{"user_id":"10002","time":1761936492340}}`
	notice := convertNotice(t, raw)

	if notice.UserID != 10002 {
		t.Errorf("用户标识为 %d", notice.UserID)
	}
	if notice.Time != 1761936492 {
		t.Errorf("时间应换算成秒，实际为 %d", notice.Time)
	}
}

// TestNoticeMissingIdentifierSkipped 校验缺少标识时跳过而不是发出一条假事件。
func TestNoticeMissingIdentifierSkipped(t *testing.T) {
	cases := []string{
		`{"header":{"eventType":"bot.followed"},"event":{}}`,
		`{"header":{"eventType":"group.join"},"event":{"chatId":"695367192"}}`,
		`{"header":{"eventType":"group.join"},"event":{"userId":"10001"}}`,
	}

	for _, raw := range cases {
		converted, ok, err := newConverter().Convert(context.Background(), parse(t, raw))
		if err != nil {
			t.Fatalf("转换失败: %v", err)
		}
		if ok || converted != nil {
			t.Errorf("缺少标识时应跳过，实际返回 %#v", converted)
		}
	}
}
