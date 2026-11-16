// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// GroupTags 查询群里的标签定义。
//
// 标签没有数字标识，名称就是唯一标识。这个接口只要求机器人在群中，不要求标签
// 管理权限，因此未授权的部署也能读到标签列表。
func (c *Client) GroupTags(ctx context.Context, groupID string) ([]model.GroupTag, error) {
	if groupID == "" {
		return nil, errors.New("查询群标签缺少群标识")
	}

	body := map[string]any{"groupId": groupID}
	data, err := c.do(ctx, http.MethodPost, "/group/tag/list", "POST /group/tag/list", nil, body)
	if err != nil {
		return nil, err
	}

	tags, err := decodeList[model.GroupTag](data)
	if err != nil {
		return nil, fmt.Errorf("解析群标签列表: %w", err)
	}

	return tags, nil
}

// CreateGroupTag 创建一个群标签。
//
// 需要机器人拥有允许控制标签组的权限，未授权时平台返回业务错误。
func (c *Client) CreateGroupTag(ctx context.Context, request model.CreateTagRequest) error {
	if request.GroupID == "" || request.Tag == "" {
		return errors.New("创建群标签缺少群标识或标签名称")
	}

	if _, err := c.do(ctx, http.MethodPost, "/group/tag/create", "POST /group/tag/create", nil, request); err != nil {
		return err
	}

	return nil
}

// EditGroupTag 修改一个群标签。
//
// 请求里没给出的字段保持不变，是否修改由请求结构决定。
func (c *Client) EditGroupTag(ctx context.Context, request model.EditTagRequest) error {
	if request.GroupID == "" || request.Tag == "" {
		return errors.New("修改群标签缺少群标识或标签名称")
	}

	if _, err := c.do(ctx, http.MethodPost, "/group/tag/edit", "POST /group/tag/edit", nil, request); err != nil {
		return err
	}

	return nil
}

// DeleteGroupTag 删除一个群标签。
//
// 删除标签之后，与该标签关联的成员关系由平台处理，本程序无法读取也无法恢复。
func (c *Client) DeleteGroupTag(ctx context.Context, groupID, tag string) error {
	if groupID == "" || tag == "" {
		return errors.New("删除群标签缺少群标识或标签名称")
	}

	body := model.TagRequest{GroupID: groupID, Tag: tag}
	if _, err := c.do(ctx, http.MethodPost, "/group/tag/delete", "POST /group/tag/delete", nil, body); err != nil {
		return err
	}

	return nil
}

// AddMemberTag 给成员加一个标签。
func (c *Client) AddMemberTag(ctx context.Context, groupID, userID, tag string) error {
	if groupID == "" || userID == "" || tag == "" {
		return errors.New("给成员加标签缺少群标识、成员标识或标签名称")
	}

	body := model.MemberTagRequest{GroupID: groupID, UserID: userID, Tag: tag}
	if _, err := c.do(ctx, http.MethodPost, "/group/tag/user-relate", "POST /group/tag/user-relate", nil, body); err != nil {
		return err
	}

	return nil
}

// RemoveMemberTag 移除成员的一个标签。
func (c *Client) RemoveMemberTag(ctx context.Context, groupID, userID, tag string) error {
	if groupID == "" || userID == "" || tag == "" {
		return errors.New("移除成员标签缺少群标识、成员标识或标签名称")
	}

	body := model.MemberTagRequest{GroupID: groupID, UserID: userID, Tag: tag}
	if _, err := c.do(ctx, http.MethodPost, "/group/tag/user-relate-cancel", "POST /group/tag/user-relate-cancel", nil, body); err != nil {
		return err
	}

	return nil
}
