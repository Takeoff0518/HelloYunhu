// SPDX-License-Identifier: GPL-3.0-or-later

package model

// MaxTagRunes 是标签名称的字符数上限。
//
// 平台按字符数计算，九个汉字可以设置成功。这里按字符数校验，而不是字节数。
const MaxTagRunes = 9

// GroupTag 是群标签的定义。
//
// 云湖的标签没有数字标识，名称就是唯一标识，改名只能调用修改接口。标签与成员的
// 关系单独存储，没有任何接口可以读回来，因此程序无法知道某个成员身上有哪些标签。
type GroupTag struct {
	// Tag 是标签名称，同时作为标识使用。
	Tag string `json:"tag"`
	// Desc 是标签描述，可以为空。
	Desc string `json:"desc"`
	// Color 是标签颜色，格式为 #RRGGBB。
	Color string `json:"color"`
	// Sort 是排序值，越小越靠前。
	Sort int64 `json:"sort"`
}

// CreateTagRequest 是 POST /group/tag/create 的请求体。
type CreateTagRequest struct {
	// GroupID 是群标识。
	GroupID string `json:"groupId"`
	// Tag 是标签名称，最长九个字符。
	Tag string `json:"tag"`
	// Color 是标签颜色，留空时由平台决定。
	Color string `json:"color,omitempty"`
	// Desc 是标签描述，留空时不设置。
	Desc string `json:"desc,omitempty"`
	// Sort 是排序值，留空时不设置。
	Sort *int64 `json:"sort,omitempty"`
}

// EditTagRequest 是 POST /group/tag/edit 的请求体。
//
// Desc 与 Sort 用指针，调用方才能区分「不改这一项」与「改成空字符串或 0」。云湖
// 允许把排序值设为 0，用零值表示未设置会让这一项无法修改。
type EditTagRequest struct {
	// GroupID 是群标识。
	GroupID string `json:"groupId"`
	// Tag 是要修改的标签名称。
	Tag string `json:"tag"`
	// NewTag 是新的标签名称，留空表示不改名。
	NewTag string `json:"newTag,omitempty"`
	// Color 是新颜色，留空表示不改。
	Color string `json:"color,omitempty"`
	// Desc 是新描述，nil 表示不改。
	Desc *string `json:"desc,omitempty"`
	// Sort 是新排序值，nil 表示不改。
	Sort *int64 `json:"sort,omitempty"`
}

// TagRequest 是删除标签的请求体。
type TagRequest struct {
	// GroupID 是群标识。
	GroupID string `json:"groupId"`
	// Tag 是标签名称。
	Tag string `json:"tag"`
}

// MemberTagRequest 是成员标签接口的请求体。
//
// 给成员加标签与移除标签用同一份结构，两者的区别只在接口路径。
type MemberTagRequest struct {
	// GroupID 是群标识。
	GroupID string `json:"groupId"`
	// UserID 是成员标识。
	UserID string `json:"userId"`
	// Tag 是标签名称。
	Tag string `json:"tag"`
}
