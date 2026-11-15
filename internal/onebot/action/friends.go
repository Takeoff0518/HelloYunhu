// SPDX-License-Identifier: GPL-3.0-or-later

package action

// friendList 处理 get_friend_list。
//
// 云湖没有提供机器人用户列表接口，官方 API 列表里的「获取机器人用户列表」标注为
// 待上线，因此这里只能返回空数组。框架遍历空列表不会出错，比返回失败更安全，也
// 不会拿虚构的用户污染会话列表。平台上线该接口后在这里查询真实数据，返回结构不变。
func (*Dispatcher) friendList() []map[string]any {
	return []map[string]any{}
}
