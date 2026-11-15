// SPDX-License-Identifier: GPL-3.0-or-later

// Package avatar 保存用户的云湖头像地址，并把头像代理给客户端下载。
//
// 云湖的头像存放在 CDN 上，请求需要附带来源头，客户端直接下载会得到 403。开启
// 兼容模式时由本程序代取，客户端只访问本程序给出的地址。头像地址从收到的消息事件
// 与查询结果里收集，因此只能提供已经见过的用户的头像，其余用户回退到占位图。
package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

// maxEntries 是地址表的容量上限。超出后随机丢弃一条，保证长期运行不会无限增长。
const maxEntries = 4096

// Store 记录整数标识与云湖头像地址的对应关系。
//
// 键是 OneBot 侧的整数标识，因为客户端的请求只带这个值。地址来自云湖返回的
// avatarUrl，不在这里做任何改写。
type Store struct {
	mu   sync.RWMutex
	urls map[int64]string
}

// NewStore 创建一张空的地址表。
func NewStore() *Store {
	return &Store{urls: make(map[int64]string)}
}

// Put 记录一个用户的头像地址。标识或地址为空时忽略，不记录空值。
func (s *Store) Put(userID int64, url string) {
	if s == nil || userID <= 0 || url == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[userID]; !exists && len(s.urls) >= maxEntries {
		// 容量已满时丢掉一条无关记录。头像会在后续事件里重新出现，
		// 随机淘汰比维护一套 LRU 更合适。
		for key := range s.urls {
			delete(s.urls, key)
			break
		}
	}
	s.urls[userID] = url
}

// Get 返回用户头像地址，没有记录时第二个返回值为 false。
func (s *Store) Get(userID int64) (string, bool) {
	if s == nil {
		return "", false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	url, ok := s.urls[userID]
	return url, ok
}

// Len 返回当前记录的条数。
func (s *Store) Len() int {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.urls)
}

// placeholderBytes 返回回退用的占位图。内容是一张纯灰的 PNG。
//
// 取不到头像时返回占位图而不是 404：客户端拿到 404 通常会抛异常或显示破图，返回
// 一张能解码的图片最稳妥。图片在首次使用时生成，不占用仓库体积。
var (
	placeholderOnce  sync.Once
	placeholderImage []byte
)

func placeholderBytes() []byte {
	placeholderOnce.Do(func() {
		const size = 64

		img := image.NewRGBA(image.Rect(0, 0, size, size))
		gray := color.RGBA{R: 204, G: 204, B: 204, A: 255}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				img.Set(x, y, gray)
			}
		}

		var buffer bytes.Buffer
		if err := png.Encode(&buffer, img); err != nil {
			return
		}
		placeholderImage = buffer.Bytes()
	})

	return placeholderImage
}
