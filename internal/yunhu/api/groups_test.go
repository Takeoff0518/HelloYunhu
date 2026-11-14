// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

func TestGroupInfo(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/info" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("groupId"); got != "20001" {
			t.Errorf("群标识参数为 %q", got)
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"groupId":"20001","groupName":"测试群","headcount":42}}`)
	})

	info, err := client.GroupInfo(context.Background(), "20001")
	if err != nil {
		t.Fatalf("GroupInfo 返回错误: %v", err)
	}
	if info.GroupName != "测试群" || info.Headcount != 42 {
		t.Errorf("群信息为 %+v", info)
	}
}

func TestBotGroupsPaginates(t *testing.T) {
	pages := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		pages++
		page := r.URL.Query().Get("page")
		if page == "1" {
			items := make([]string, 0, defaultPageSize)
			for range defaultPageSize {
				items = append(items, `{"groupId":"1","groupName":"满页"}`)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[`+strings.Join(items, ",")+`]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"groupId":"2","groupName":"第二页"}]}}`)
	})

	groups, err := client.BotGroups(context.Background())
	if err != nil {
		t.Fatalf("BotGroups 返回错误: %v", err)
	}
	if len(groups) != defaultPageSize+1 {
		t.Errorf("群数量为 %d，期望 %d", len(groups), defaultPageSize+1)
	}
	if pages != 2 {
		t.Errorf("请求页数为 %d，期望 2", pages)
	}
	if groups[len(groups)-1].GroupName != "第二页" {
		t.Errorf("最后一页为 %+v", groups[len(groups)-1])
	}
}

func TestBotGroupsAcceptsDirectArray(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":[{"groupId":"1","groupName":"群一"}]}`)
	})

	groups, err := client.BotGroups(context.Background())
	if err != nil {
		t.Fatalf("BotGroups 返回错误: %v", err)
	}
	if len(groups) != 1 || groups[0].GroupName != "群一" {
		t.Errorf("群列表为 %+v", groups)
	}
}

func TestGroupMembers(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/members" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"members":[{"userId":"10001","nickname":"小明","role":"member"}]}}`)
	})

	members, err := client.GroupMembers(context.Background(), "20001")
	if err != nil {
		t.Fatalf("GroupMembers 返回错误: %v", err)
	}
	if len(members) != 1 || members[0].UserID != "10001" || members[0].Nickname != "小明" {
		t.Errorf("成员列表为 %+v", members)
	}
}

func TestUploadImage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/image/upload" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析上传表单失败: %v", err)
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("读取上传文件失败: %v", err)
		}
		defer func() {
			_ = file.Close()
		}()

		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("读取上传内容失败: %v", err)
		}
		if string(content) != "fake-image" {
			t.Errorf("上传内容为 %q", content)
		}
		if header.Filename != "a.png" {
			t.Errorf("文件名为 %q", header.Filename)
		}

		_, _ = io.WriteString(w, `{"code":0,"data":{"imageKey":"key-1"}}`)
	})

	key, err := client.UploadImage(context.Background(), "a.png", []byte("fake-image"))
	if err != nil {
		t.Fatalf("UploadImage 返回错误: %v", err)
	}
	if key != "key-1" {
		t.Errorf("图片标识为 %q", key)
	}
}

func TestUploadImageRejectsOversize(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("超过上限时不应发起请求")
	})

	data := make([]byte, model.MaxImageBytes+1)
	if _, err := client.UploadImage(context.Background(), "a.png", data); err == nil {
		t.Fatal("超过上限应返回错误")
	}
}

func TestUploadImageRejectsEmpty(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("空内容不应发起请求")
	})

	if _, err := client.UploadImage(context.Background(), "a.png", nil); err == nil {
		t.Fatal("空内容应返回错误")
	}
}

func TestUploadImageMissingKey(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":0,"data":{}}`)
	})

	_, err := client.UploadImage(context.Background(), "a.png", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "imageKey") {
		t.Fatalf("错误为 %v，期望说明缺少 imageKey", err)
	}
}
