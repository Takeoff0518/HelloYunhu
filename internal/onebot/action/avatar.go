// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Takeoff0518/HelloYunhu/internal/avatar"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// defaultAppName 是未配置时向框架自报的程序名。
const defaultAppName = "HelloYunhu"

// appName 返回生效的程序名。
func (d *Dispatcher) appName() string {
	if d.AppName == "" {
		return defaultAppName
	}
	return d.AppName
}

// getAvatar 处理非标准的 get_avatar。
//
// 这个接口来自 Gensokyo 的扩展，参数是 user_id 与可选的 group_id，返回值是头像
// 地址本身，不是对象。只有在兼容模式下才提供，其余情况按未实现处理。
func (d *Dispatcher) getAvatar(params json.RawMessage) (any, error) {
	if d.AvatarBaseURL == "" {
		return nil, obmodel.NotImplemented("get_avatar")
	}

	var value struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(params, &value); err != nil {
		return nil, badRequest("get_avatar", err)
	}
	if value.UserID <= 0 {
		return nil, &obmodel.ActionError{
			Retcode: obmodel.RetcodeBadRequest,
			Wording: "get_avatar 缺少 user_id",
		}
	}

	base := strings.TrimRight(d.AvatarBaseURL, "/")
	return base + avatar.PathPrefix + strconv.FormatInt(value.UserID, 10), nil
}

// rememberAvatar 记录成员头像，供头像代理取图。
//
// 头像地址只出现在云湖返回的查询结果与消息事件里，没有单独的查询接口，因此见过
// 谁的资料才认识谁的头像，其余用户由代理回退到占位图。
func (d *Dispatcher) rememberAvatar(member yunhumodel.GroupMember) {
	if d.Avatars == nil || member.AvatarURL == "" {
		return
	}

	userID, exact := identity.ParseID(member.UserID)
	if !exact {
		return
	}
	d.Avatars.Put(userID, member.AvatarURL)
}
