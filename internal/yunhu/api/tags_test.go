// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

func TestGroupTags(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/tag/list" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("请求方法为 %q", r.Method)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if body["groupId"] != "20001" {
			t.Errorf("群标识为 %v", body["groupId"])
		}

		_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{"list":[
			{"tag":"VIP用户","desc":"会员","color":"#FF5733","sort":1},
			{"tag":"活跃用户","desc":"","color":"#33C4FF","sort":2}]}}`)
	})

	tags, err := client.GroupTags(context.Background(), "20001")
	if err != nil {
		t.Fatalf("GroupTags 返回错误: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("标签数量为 %d", len(tags))
	}
	if tags[0].Tag != "VIP用户" || tags[0].Color != "#FF5733" || tags[0].Sort != 1 {
		t.Errorf("第一个标签为 %+v", tags[0])
	}
}

func TestCreateGroupTagOmitsUnsetFields(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/tag/create" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}

		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		// 没有设置的可选项不出现，避免把平台字段覆盖成零值。
		for _, key := range []string{"color", "desc", "sort"} {
			if _, ok := body[key]; ok {
				t.Errorf("未设置时不应发送 %s", key)
			}
		}

		_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)
	})

	if err := client.CreateGroupTag(context.Background(), model.CreateTagRequest{GroupID: "20001", Tag: "VIP"}); err != nil {
		t.Fatalf("CreateGroupTag 返回错误: %v", err)
	}
}

func TestEditGroupTagKeepsZeroValues(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/tag/edit" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}

		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}

		// 排序值 0 与空描述都是有效取值，必须原样发出。
		if string(body["sort"]) != "0" {
			t.Errorf("排序值为 %s", body["sort"])
		}
		if string(body["desc"]) != `""` {
			t.Errorf("描述为 %s", body["desc"])
		}
		if _, ok := body["newTag"]; ok {
			t.Error("没有给新名称时不应发送 newTag")
		}

		_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)
	})

	sort := int64(0)
	desc := ""
	request := model.EditTagRequest{GroupID: "20001", Tag: "VIP", Desc: &desc, Sort: &sort}
	if err := client.EditGroupTag(context.Background(), request); err != nil {
		t.Fatalf("EditGroupTag 返回错误: %v", err)
	}
}

func TestMemberTagRequests(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(client *Client) error
	}{
		{
			name: "加标签",
			path: "/open-apis/v1/group/tag/user-relate",
			call: func(client *Client) error {
				return client.AddMemberTag(context.Background(), "20001", "2272345", "VIP")
			},
		},
		{
			name: "移除标签",
			path: "/open-apis/v1/group/tag/user-relate-cancel",
			call: func(client *Client) error {
				return client.RemoveMemberTag(context.Background(), "20001", "2272345", "VIP")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("请求路径为 %q", r.URL.Path)
				}

				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("解析请求体失败: %v", err)
				}
				if body["groupId"] != "20001" || body["userId"] != "2272345" || body["tag"] != "VIP" {
					t.Errorf("请求内容为 %v", body)
				}

				_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)
			})

			if err := tt.call(client); err != nil {
				t.Fatalf("调用返回错误: %v", err)
			}
		})
	}
}

func TestDeleteGroupTag(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/v1/group/tag/delete" {
			t.Errorf("请求路径为 %q", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("解析请求体失败: %v", err)
		}
		if body["tag"] != "VIP" || body["groupId"] != "20001" {
			t.Errorf("请求内容为 %v", body)
		}

		_, _ = io.WriteString(w, `{"code":1,"msg":"success","data":{}}`)
	})

	if err := client.DeleteGroupTag(context.Background(), "20001", "VIP"); err != nil {
		t.Fatalf("DeleteGroupTag 返回错误: %v", err)
	}
}
