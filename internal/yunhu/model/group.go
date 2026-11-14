// SPDX-License-Identifier: GPL-3.0-or-later

package model

// GroupInfo 是 GET /group/info 返回的群信息。
//
// 字段名按调研报告记录的 headcount 与 permissions 组织，其余字段首版不使用。
type GroupInfo struct {
	// GroupID 是群标识。
	GroupID string `json:"groupId"`
	// GroupName 是群名称。
	GroupName string `json:"groupName"`
	// Headcount 是群成员数量。
	Headcount int `json:"headcount"`
}

// GroupSummary 是 GET /bot/groups 返回的群摘要。
type GroupSummary struct {
	GroupID   string `json:"groupId"`
	GroupName string `json:"groupName"`
	Headcount int    `json:"headcount"`
}

// GroupMember 是 GET /group/members 返回的群成员。
type GroupMember struct {
	// UserID 是成员标识。
	UserID string `json:"userId"`
	// Nickname 是成员昵称。
	Nickname string `json:"nickname"`
	// Card 是群名片，没有设置时为空。
	Card string `json:"card"`
	// Role 是成员角色，取值为 owner、admin 或 member。
	Role string `json:"role"`
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
