// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// UploadImage 上传图片并返回图片标识。
//
// 云湖不接受直接传网络地址，图片必须先上传取得 imageKey 才能发送。
func (c *Client) UploadImage(ctx context.Context, name string, data []byte) (string, error) {
	result, err := c.upload(ctx, "/image/upload", "POST /image/upload", "image", name, data, model.MaxImageBytes)
	if err != nil {
		return "", err
	}
	if result.ImageKey == "" {
		return "", errors.New("图片上传结果缺少 imageKey")
	}
	return result.ImageKey, nil
}

// UploadFile 上传文件并返回文件标识。
func (c *Client) UploadFile(ctx context.Context, name string, data []byte) (string, error) {
	result, err := c.upload(ctx, "/file/upload", "POST /file/upload", "file", name, data, model.MaxFileBytes)
	if err != nil {
		return "", err
	}
	if result.FileKey == "" {
		return "", errors.New("文件上传结果缺少 fileKey")
	}
	return result.FileKey, nil
}

// UploadVideo 上传视频并返回视频标识。
func (c *Client) UploadVideo(ctx context.Context, name string, data []byte) (string, error) {
	result, err := c.upload(ctx, "/video/upload", "POST /video/upload", "video", name, data, model.MaxVideoBytes)
	if err != nil {
		return "", err
	}
	if result.VideoKey == "" {
		return "", errors.New("视频上传结果缺少 videoKey")
	}
	return result.VideoKey, nil
}

// upload 上传一份媒体内容。
func (c *Client) upload(ctx context.Context, path, op, field, name string, data []byte, limit int) (*model.UploadResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: 媒体内容为空", op)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s: 媒体体积 %d 字节超过上限 %d 字节", op, len(data), limit)
	}

	name = fileName(name, data)

	endpoint, err := c.endpoint(path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile(field, name)
	if err != nil {
		return nil, fmt.Errorf("%s: 构造上传表单: %w", op, err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("%s: 写入上传内容: %w", op, err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("%s: 结束上传表单: %w", op, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("%s: 构造请求: %w", op, err)
	}
	// execute 会覆盖 Accept，这里只设置上传需要的类型。
	request.Header.Set("Content-Type", writer.FormDataContentType())

	payload, err := c.execute(request, op)
	if err != nil {
		return nil, err
	}

	var result model.UploadResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("%s: 解析上传结果: %w", op, err)
	}

	return &result, nil
}
