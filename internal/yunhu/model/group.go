// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "encoding/json"

// GroupInfo 是 GET /group/info 返回的群信息。
//
// 平台返回的群名字段是 name，早期按 groupName 实现，实测后两种写法都接受。头像、
// 简介、封禁状态等字段首版不使用。
type GroupInfo struct {
	// GroupID 是群标识。
	GroupID string `json:"-"`
	// GroupName 是群名称。
	GroupName string `json:"-"`
	// Headcount 是群成员数量。
	Headcount int `json:"-"`
	// Permissions 是机器人在这个群获得的管理权限，机器人不在群中时全为假。
	Permissions Permissions `json:"-"`
}

// UnmarshalJSON 解析群信息，群名称同时接受 name 与 groupName。
func (g *GroupInfo) UnmarshalJSON(raw []byte) error {
	var value struct {
		GroupID     string      `json:"groupId"`
		GroupName   string      `json:"groupName"`
		Name        string      `json:"name"`
		Headcount   int         `json:"headcount"`
		Permissions Permissions `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}

	g.GroupID = value.GroupID
	g.Headcount = value.Headcount
	g.Permissions = value.Permissions
	g.GroupName = value.GroupName
	if g.GroupName == "" {
		g.GroupName = value.Name
	}

	return nil
}

// GroupSummary 是 GET /bot/groups 返回的群摘要。
//
// 列表里的字段与群信息一致，只有群标识、群名称与头像，没有成员数量。
type GroupSummary struct {
	GroupID   string `json:"-"`
	GroupName string `json:"-"`
	Headcount int    `json:"-"`
}

// UnmarshalJSON 解析群摘要，群名称同时接受 name 与 groupName。
func (g *GroupSummary) UnmarshalJSON(raw []byte) error {
	var value struct {
		GroupID   string `json:"groupId"`
		GroupName string `json:"groupName"`
		Name      string `json:"name"`
		Headcount int    `json:"headcount"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}

	g.GroupID = value.GroupID
	g.Headcount = value.Headcount
	g.GroupName = value.GroupName
	if g.GroupName == "" {
		g.GroupName = value.Name
	}

	return nil
}

// GroupMember 是 GET /group/members 返回的群成员。
//
// 实测字段为 userId、nickname、groupNickname 与 userLevel，其中 userLevel 取
// owner、admin 或 member。接口需要机器人在群后台获得成员列表权限，未授权时返回
// 1003，调用方需要降级处理。
type GroupMember struct {
	// UserID 是成员标识。
	UserID string `json:"-"`
	// Nickname 是成员昵称。
	Nickname string `json:"-"`
	// Card 是群名片，没有设置时为空。
	Card string `json:"-"`
	// Role 是成员角色，取值为 owner、admin 或 member。
	Role string `json:"-"`
	// AvatarURL 是成员头像地址，位于云湖的图片 CDN 上。
	AvatarURL string `json:"-"`
}

// UnmarshalJSON 解析群成员，同时接受实测字段与几种常见写法。
func (m *GroupMember) UnmarshalJSON(raw []byte) error {
	var value struct {
		UserID    string `json:"userId"`
		SenderID  string `json:"senderId"`
		ID        string `json:"id"`
		Nickname  string `json:"nickname"`
		NickName  string `json:"nickName"`
		GroupName string `json:"groupNickname"`
		Card      string `json:"card"`
		UserLevel string `json:"userLevel"`
		Role      string `json:"role"`
		AvatarURL string `json:"avatarUrl"`
		AvatarAlt string `json:"avatar_url"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}

	m.UserID = firstNonEmpty(value.UserID, value.SenderID, value.ID)
	m.Nickname = firstNonEmpty(value.Nickname, value.NickName)
	m.Card = firstNonEmpty(value.Card, value.GroupName)
	m.Role = firstNonEmpty(value.Role, value.UserLevel)
	m.AvatarURL = firstNonEmpty(value.AvatarURL, value.AvatarAlt)

	return nil
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// UploadResult 是媒体上传接口返回的标识。
type UploadResult struct {
	ImageKey string `json:"imageKey"`
	FileKey  string `json:"fileKey"`
	VideoKey string `json:"videoKey"`
}

// 媒体体积上限，单位字节。平台限制图片 10 MB，文件与视频 20 MB。
const (
	MaxImageBytes = 10 << 20
	MaxFileBytes  = 20 << 20
	MaxVideoBytes = 20 << 20
)
