// SPDX-License-Identifier: GPL-3.0-or-later

// Package proxypath 定义代理服务的路径前缀。
//
// 前缀由代理服务注册、由事件转换拼接，两处必须一致，因此单独放在一个包里，
// 避免各写一份字面量。
package proxypath

const (
	// Avatar 是头像路径的前缀，后面跟整数标识。
	Avatar = "/avatar/"
	// Media 是媒体路径的前缀，后面跟原始地址的 base64url 编码。
	Media = "/media/"
)
