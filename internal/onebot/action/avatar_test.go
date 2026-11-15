// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/avatar"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// actionError 把处理结果里的错误转成 ActionError，判断响应码用。
func actionError(t *testing.T, err error) *obmodel.ActionError {
	t.Helper()

	var target *obmodel.ActionError
	if !errors.As(err, &target) {
		t.Fatalf("错误为 %v，期望 ActionError", err)
	}
	return target
}

func TestVersionInfoAppName(t *testing.T) {
	cases := []struct {
		name    string
		appName string
		want    string
	}{
		{name: "未配置时用本项目的名字", appName: "", want: "HelloYunhu"},
		{name: "兼容模式自报另一个项目", appName: "gensokyo", want: "gensokyo"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dispatcher := &Dispatcher{AppName: c.appName}

			data, err := dispatcher.Handle(context.Background(), "get_version_info", nil)
			if err != nil {
				t.Fatalf("处理失败: %v", err)
			}
			if got := data.(map[string]any)["app_name"]; got != c.want {
				t.Errorf("app_name 为 %v，期望 %s", got, c.want)
			}
		})
	}
}

func TestGetAvatar(t *testing.T) {
	dispatcher := &Dispatcher{AvatarBaseURL: "http://127.0.0.1:8082/"}

	data, err := dispatcher.Handle(context.Background(), "get_avatar", json.RawMessage(`{"user_id":10001,"group_id":20001}`))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	url, ok := data.(string)
	if !ok {
		t.Fatalf("返回值类型为 %T，期望字符串", data)
	}
	if url != "http://127.0.0.1:8082/avatar/10001" {
		t.Errorf("头像地址为 %q", url)
	}
}

func TestGetAvatarDisabled(t *testing.T) {
	dispatcher := &Dispatcher{}

	_, err := dispatcher.Handle(context.Background(), "get_avatar", json.RawMessage(`{"user_id":10001}`))
	if got := actionError(t, err).Retcode; got != obmodel.RetcodeNotFound {
		t.Errorf("响应码为 %d，期望 1404", got)
	}
}

func TestGetAvatarMissingUser(t *testing.T) {
	dispatcher := &Dispatcher{AvatarBaseURL: "http://127.0.0.1:8082"}

	_, err := dispatcher.Handle(context.Background(), "get_avatar", json.RawMessage(`{}`))
	if got := actionError(t, err).Retcode; got != obmodel.RetcodeBadRequest {
		t.Errorf("响应码为 %d，期望 100", got)
	}
}

func TestRememberAvatar(t *testing.T) {
	store := avatar.NewStore()
	dispatcher := &Dispatcher{Avatars: store}

	dispatcher.rememberAvatar(yunhumodel.GroupMember{
		UserID:    "10001",
		AvatarURL: "https://chat-img.jwznb.com/a.jpg",
	})

	url, ok := store.Get(10001)
	if !ok || url != "https://chat-img.jwznb.com/a.jpg" {
		t.Fatalf("记录的头像地址为 %q、%v", url, ok)
	}

	// 没有地址表时不应 panic，兼容模式关闭时走的就是这条路径。
	(&Dispatcher{}).rememberAvatar(yunhumodel.GroupMember{UserID: "10001", AvatarURL: "https://example.com/a.jpg"})
}
