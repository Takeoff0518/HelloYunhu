// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "encoding/json"

// GroupInfo 是 GET /group/info 返回的群信息。
//
// 平台返回的群名字段是 name，早期按 groupName 实现，实测后两种写法都接受。其余
// 字段首版不使用。
type GroupInfo struct {
	// GroupID 是群标识。
	GroupID string `json:"-"`
	// GroupName 是群名称。
	GroupName string `json:"-"`
	// Headcount 是群成员数量。
	Headcount int `json:"-"`
}

// UnmarshalJSON 解析群信息，群名称同时接受 name 与 groupName。
func (g *GroupInfo) UnmarshalJSON(raw []byte) error {
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
// 平台对该接口默认关闭，机器人没有权限时返回 1003，调用方需要降级处理。字段名
// 尚未在真实环境确认，解析时接受常见写法。
type GroupMember struct {
	// UserID 是成员标识。
	UserID string `json:"-"`
	// Nickname 是成员昵称。
	Nickname string `json:"-"`
	// Card 是群名片，没有设置时为空。
	Card string `json:"-"`
	// Role 是成员角色，取值为 owner、admin 或 member。
	Role string `json:"-"`
}

// UnmarshalJSON 解析群成员，标识与昵称同时接受几种常见写法。
func (m *GroupMember) UnmarshalJSON(raw []byte) error {
	var value struct {
		UserID   string `json:"userId"`
		SenderID string `json:"senderId"`
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
		NickName string `json:"nickName"`
		Card     string `json:"card"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}

	m.UserID = firstNonEmpty(value.UserID, value.SenderID, value.ID)
	m.Nickname = firstNonEmpty(value.Nickname, value.NickName)
	m.Card = value.Card
	m.Role = value.Role

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
