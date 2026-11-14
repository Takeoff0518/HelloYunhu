// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// 分页参数。群列表与成员列表都按分页返回，一次最多一百条。
const (
	defaultPageSize = 100
	maxPages        = 50
)

// GroupInfo 查询群信息。
func (c *Client) GroupInfo(ctx context.Context, groupID string) (*model.GroupInfo, error) {
	if groupID == "" {
		return nil, fmt.Errorf("群标识不能为空")
	}

	query := url.Values{"groupId": {groupID}}
	data, err := c.do(ctx, http.MethodGet, "/group/info", "GET /group/info", query, nil)
	if err != nil {
		return nil, err
	}

	var info model.GroupInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析群信息: %w", err)
	}
	if info.GroupID == "" {
		info.GroupID = groupID
	}

	return &info, nil
}

// BotGroups 查询机器人所在的群，自动翻页直到取满。
func (c *Client) BotGroups(ctx context.Context) ([]model.GroupSummary, error) {
	var groups []model.GroupSummary

	for page := 1; page <= maxPages; page++ {
		query := url.Values{
			"page": {fmt.Sprintf("%d", page)},
			"size": {fmt.Sprintf("%d", defaultPageSize)},
		}

		data, err := c.do(ctx, http.MethodGet, "/bot/groups", "GET /bot/groups", query, nil)
		if err != nil {
			return nil, err
		}

		items, err := decodeList[model.GroupSummary](data)
		if err != nil {
			return nil, fmt.Errorf("解析群列表: %w", err)
		}
		groups = append(groups, items...)

		if len(items) < defaultPageSize {
			break
		}
	}

	return groups, nil
}

// GroupMembers 查询群成员，自动翻页直到取满。
//
// 群成员权限默认关闭，没有权限时平台返回业务错误，调用方需要降级而不是报错。
func (c *Client) GroupMembers(ctx context.Context, groupID string) ([]model.GroupMember, error) {
	if groupID == "" {
		return nil, fmt.Errorf("群标识不能为空")
	}

	var members []model.GroupMember

	for page := 1; page <= maxPages; page++ {
		query := url.Values{
			"groupId": {groupID},
			"page":    {fmt.Sprintf("%d", page)},
			"size":    {fmt.Sprintf("%d", defaultPageSize)},
		}

		data, err := c.do(ctx, http.MethodGet, "/group/members", "GET /group/members", query, nil)
		if err != nil {
			return nil, err
		}

		items, err := decodeList[model.GroupMember](data)
		if err != nil {
			return nil, fmt.Errorf("解析群成员列表: %w", err)
		}
		members = append(members, items...)

		if len(items) < defaultPageSize {
			break
		}
	}

	return members, nil
}

// decodeList 解析列表响应。
//
// 平台没有给出确切的列表结构，可能直接返回数组，也可能包在 list、items 等字段里。
// 这里两种形态都接受，未知形态按空列表处理，字段名在实测后需要收敛。
func decodeList[T any](raw json.RawMessage) ([]T, error) {
	var direct []T
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}

	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}

	for _, key := range []string{"list", "items", "members", "groups", "data"} {
		value, ok := wrapped[key]
		if !ok {
			continue
		}
		var items []T
		if err := json.Unmarshal(value, &items); err != nil {
			return nil, fmt.Errorf("字段 %s: %w", key, err)
		}
		return items, nil
	}

	return nil, nil
}
