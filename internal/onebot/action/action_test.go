// SPDX-License-Identifier: GPL-3.0-or-later

package action

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cache"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
	"github.com/Takeoff0518/HelloYunhu/internal/bridge/media"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/api"
	yunhumodel "github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

// sentRequest 记录测试服务收到的发送请求。
type sentRequest struct {
	mu       sync.Mutex
	requests []yunhumodel.SendMessageRequest
}

// newDispatcher 启动模拟的云湖接口服务并返回处理器。
func newDispatcher(t *testing.T) (*Dispatcher, *sentRequest) {
	t.Helper()

	recorder := &sentRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/v1/bot/send":
			var request yunhumodel.SendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("解析请求失败: %v", err)
			}
			recorder.mu.Lock()
			recorder.requests = append(recorder.requests, request)
			recorder.mu.Unlock()

			_, _ = io.WriteString(w, `{"code":0,"data":{"msgId":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}`)

		case "/open-apis/v1/group/info":
			_, _ = io.WriteString(w, `{"code":0,"data":{"groupId":"20001","groupName":"测试群","headcount":42}}`)

		case "/open-apis/v1/bot/groups":
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"groupId":"20001","groupName":"测试群","headcount":42}]}}`)

		case "/open-apis/v1/group/members":
			_, _ = io.WriteString(w, `{"code":0,"data":{"members":[{"userId":"10001","nickname":"小明","role":"admin"}]}}`)

		case "/open-apis/v1/image/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析上传表单失败: %v", err)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"imageKey":"img-key"}}`)

		case "/open-apis/v1/file/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析上传表单失败: %v", err)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"fileKey":"file-key"}}`)

		case "/open-apis/v1/video/upload":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("解析上传表单失败: %v", err)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"videoKey":"video-key"}}`)

		default:
			t.Errorf("请求路径为 %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := api.New(api.Config{BaseURL: server.URL + "/open-apis/v1", Token: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	return &Dispatcher{
		SelfID:   1001,
		Nickname: "机器人",
		API:      client,
		Messages: identity.NewMemory(),
		Media:    &media.Loader{HTTPClient: server.Client()},
	}, recorder
}

// last 返回最后一次发送请求。
func (r *sentRequest) last(t *testing.T) yunhumodel.SendMessageRequest {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.requests) == 0 {
		t.Fatal("没有收到发送请求")
	}
	return r.requests[len(r.requests)-1]
}

// textParams 构造发送参数。
func textParams(t *testing.T, extra map[string]any, text string) json.RawMessage {
	t.Helper()

	params := map[string]any{
		"message": []map[string]any{
			{"type": "text", "data": map[string]any{"text": text}},
		},
	}
	for key, value := range extra {
		params[key] = value
	}

	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}
	return raw
}

func TestSendGroupMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	data, err := dispatcher.Handle(context.Background(), "send_group_msg", textParams(t, map[string]any{"group_id": 20001}, "你好"))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	if request.RecvType != yunhumodel.RecvTypeGroup || request.RecvID != "20001" {
		t.Errorf("接收方为 %s/%s", request.RecvType, request.RecvID)
	}
	if request.ContentType != yunhumodel.ContentTypeText {
		t.Errorf("内容类型为 %q", request.ContentType)
	}

	content := request.Content.(map[string]any)
	if content["text"] != "你好" {
		t.Errorf("正文为 %v", content["text"])
	}

	payload := data.(map[string]any)
	if payload["message_id"] != int64(1) {
		t.Errorf("消息标识为 %v", payload["message_id"])
	}
}

func TestSendPrivateMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	if _, err := dispatcher.Handle(context.Background(), "send_private_msg", textParams(t, map[string]any{"user_id": 10001}, "在吗")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	if request.RecvType != yunhumodel.RecvTypeUser || request.RecvID != "10001" {
		t.Errorf("接收方为 %s/%s", request.RecvType, request.RecvID)
	}
}

func TestSendMessageRoutesByType(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	if _, err := dispatcher.Handle(context.Background(), "send_msg", textParams(t, map[string]any{"message_type": "group", "group_id": 20001}, "群聊")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if got := recorder.last(t).RecvType; got != yunhumodel.RecvTypeGroup {
		t.Errorf("接收方类型为 %q", got)
	}

	if _, err := dispatcher.Handle(context.Background(), "send_msg", textParams(t, map[string]any{"message_type": "private", "user_id": 10001}, "私聊")); err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if got := recorder.last(t).RecvType; got != yunhumodel.RecvTypeUser {
		t.Errorf("接收方类型为 %q", got)
	}
}

func TestSendIncludesAt(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"at","data":{"qq":10002}},{"type":"text","data":{"text":" 你好"}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	content := request.Content.(map[string]any)
	at, ok := content["at"].([]any)
	if !ok || len(at) != 1 || at[0] != "10002" {
		t.Errorf("at 列表为 %#v", content["at"])
	}
}

func TestSendReplyUsesParentID(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	// 先注册一条收到的消息，模拟框架引用它。
	messageID, err := dispatcher.Messages.MessageID(context.Background(), "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("注册消息标识失败: %v", err)
	}

	params := map[string]any{
		"group_id": 20001,
		"message": []map[string]any{
			{"type": "reply", "data": map[string]any{"id": messageID}},
			{"type": "text", "data": map[string]any{"text": "回复"}},
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}

	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", raw); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	if got := recorder.last(t).ParentID; got != "0f1e2d3c4b5a69788796a5b4c3d2e1f0" {
		t.Errorf("引用目标为 %q", got)
	}
}

func TestSendReplyWithUnknownIDIsIgnored(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"reply","data":{"id":999}},{"type":"text","data":{"text":"回复"}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	if got := recorder.last(t).ParentID; got != "" {
		t.Errorf("引用目标应为空，实际为 %q", got)
	}
}

func TestSendImageUploadsFirst(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := map[string]any{
		"group_id": 20001,
		"message": []map[string]any{
			{"type": "image", "data": map[string]any{"file": "base64://" + base64.StdEncoding.EncodeToString([]byte("image-bytes"))}},
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}

	data, err := dispatcher.Handle(context.Background(), "send_group_msg", raw)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	request := recorder.last(t)
	if request.ContentType != yunhumodel.ContentTypeImage {
		t.Errorf("内容类型为 %q", request.ContentType)
	}
	content, ok := request.Content.(map[string]any)
	if !ok || content["imageKey"] != "img-key" {
		t.Errorf("内容为 %#v", request.Content)
	}
	if payload, ok := data.(map[string]any); !ok || payload["message_id"] != int64(1) {
		t.Errorf("返回数据为 %#v", data)
	}
}

func TestSendImageWithoutLoader(t *testing.T) {
	dispatcher, _ := newDispatcher(t)
	dispatcher.Media = nil

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"image","data":{"file":"base64://eA=="}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err == nil {
		t.Fatal("没有媒体读取器时应返回错误")
	}
}

func TestSendFileAndVideoUploadFirst(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	fileParams := json.RawMessage(`{"group_id":20001,"message":[{"type":"file","data":{"file":"base64://ZmlsZQ==","name":"report.pdf"}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", fileParams); err != nil {
		t.Fatalf("发送文件失败: %v", err)
	}

	fileRequest := recorder.last(t)
	if fileRequest.ContentType != yunhumodel.ContentTypeFile {
		t.Errorf("文件的内容类型为 %q", fileRequest.ContentType)
	}
	fileContent := fileRequest.Content.(map[string]any)
	if fileContent["fileKey"] != "file-key" || fileContent["fileName"] != "report.pdf" {
		t.Errorf("文件内容为 %#v", fileContent)
	}

	videoParams := json.RawMessage(`{"group_id":20001,"message":[{"type":"video","data":{"file":"base64://dmlkZW8="}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", videoParams); err != nil {
		t.Fatalf("发送视频失败: %v", err)
	}

	videoRequest := recorder.last(t)
	if videoRequest.ContentType != yunhumodel.ContentTypeVideo {
		t.Errorf("视频的内容类型为 %q", videoRequest.ContentType)
	}
	if videoRequest.Content.(map[string]any)["videoKey"] != "video-key" {
		t.Errorf("视频内容为 %#v", videoRequest.Content)
	}
}

func TestSendRecordBecomesFile(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":[{"type":"record","data":{"file":"base64://YXVkaW8="}}]}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("发送语音失败: %v", err)
	}

	request := recorder.last(t)
	if request.ContentType != yunhumodel.ContentTypeFile {
		t.Errorf("语音应按文件发送，内容类型为 %q", request.ContentType)
	}
	if request.Content.(map[string]any)["fileKey"] != "file-key" {
		t.Errorf("内容为 %#v", request.Content)
	}
}

func TestGetMsg(t *testing.T) {
	dispatcher, _ := newDispatcher(t)
	dispatcher.Recent = cache.NewMessages(4)

	event := map[string]any{"post_type": "message", "message_id": 7}
	dispatcher.Recent.Put(7, event)

	data, err := dispatcher.Handle(context.Background(), "get_msg", json.RawMessage(`{"message_id":7}`))
	if err != nil {
		t.Fatalf("get_msg 失败: %v", err)
	}
	if got, ok := data.(map[string]any); !ok || got["message_id"] != 7 {
		t.Errorf("回放的事件为 %#v", data)
	}
}

func TestGetMsgMissReturnsNotFound(t *testing.T) {
	dispatcher, _ := newDispatcher(t)
	dispatcher.Recent = cache.NewMessages(4)

	_, err := dispatcher.Handle(context.Background(), "get_msg", json.RawMessage(`{"message_id":99}`))

	var actionErr *obmodel.ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("错误类型为 %T", err)
	}
	if actionErr.Retcode != obmodel.RetcodeNotFound {
		t.Errorf("返回码为 %d，期望 1404", actionErr.Retcode)
	}
}

func TestGetMsgWithoutCache(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	if _, err := dispatcher.Handle(context.Background(), "get_msg", json.RawMessage(`{"message_id":1}`)); err == nil {
		t.Fatal("没有缓存时应返回错误")
	}
}

func TestSendCQStringMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	params := json.RawMessage(`{"group_id":20001,"message":"[CQ:at,qq=10002] 你好"}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	request := recorder.last(t)
	if request.ContentType != yunhumodel.ContentTypeText {
		t.Errorf("内容类型为 %q", request.ContentType)
	}
	content := request.Content.(map[string]any)
	if at, ok := content["at"].([]any); !ok || len(at) != 1 || at[0] != "10002" {
		t.Errorf("at 列表为 %#v", content["at"])
	}
	if text, _ := content["text"].(string); !strings.Contains(text, "你好") {
		t.Errorf("正文为 %q", content["text"])
	}
}

func TestSendBadParams(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	tests := []struct {
		name   string
		action string
		params string
	}{
		{name: "缺少群标识", action: "send_group_msg", params: `{"message":[]}`},
		{name: "缺少用户标识", action: "send_private_msg", params: `{"message":[]}`},
		{name: "消息类型不支持", action: "send_msg", params: `{"message_type":"discuss","message":[]}`},
		{name: "参数无法解析", action: "send_group_msg", params: `{"group_id":"abc"}`},
		{name: "空消息", action: "send_group_msg", params: `{"group_id":20001,"message":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dispatcher.Handle(context.Background(), tt.action, json.RawMessage(tt.params))

			var actionErr *obmodel.ActionError
			if !errors.As(err, &actionErr) {
				t.Fatalf("错误类型为 %T: %v", err, err)
			}
			if actionErr.Retcode != obmodel.RetcodeBadRequest {
				t.Errorf("返回码为 %d，期望 %d", actionErr.Retcode, obmodel.RetcodeBadRequest)
			}
		})
	}
}

func TestUnimplementedAction(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	_, err := dispatcher.Handle(context.Background(), "set_group_whole_ban", json.RawMessage(`{}`))

	var actionErr *obmodel.ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("错误类型为 %T", err)
	}
	if actionErr.Retcode != obmodel.RetcodeNotFound {
		t.Errorf("返回码为 %d，期望 1404", actionErr.Retcode)
	}
	if !strings.Contains(actionErr.Wording, "set_group_whole_ban") {
		t.Errorf("说明为 %q", actionErr.Wording)
	}
}

func TestGroupActions(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	info, err := dispatcher.Handle(context.Background(), "get_group_info", json.RawMessage(`{"group_id":20001}`))
	if err != nil {
		t.Fatalf("get_group_info 失败: %v", err)
	}
	group := info.(map[string]any)
	if group["group_name"] != "测试群" || group["member_count"] != 42 {
		t.Errorf("群信息为 %v", group)
	}

	list, err := dispatcher.Handle(context.Background(), "get_group_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_group_list 失败: %v", err)
	}
	groups, ok := list.([]map[string]any)
	if !ok || len(groups) != 1 || groups[0]["group_id"] != int64(20001) {
		t.Errorf("群列表为 %#v", list)
	}

	member, err := dispatcher.Handle(context.Background(), "get_group_member_info", json.RawMessage(`{"group_id":20001,"user_id":10001}`))
	if err != nil {
		t.Fatalf("get_group_member_info 失败: %v", err)
	}
	person := member.(map[string]any)
	if person["nickname"] != "小明" || person["role"] != "admin" {
		t.Errorf("成员信息为 %v", person)
	}

	stranger, err := dispatcher.Handle(context.Background(), "get_stranger_info", json.RawMessage(`{"user_id":10001}`))
	if err != nil {
		t.Fatalf("get_stranger_info 失败: %v", err)
	}
	if stranger.(map[string]any)["user_id"] != int64(10001) {
		t.Errorf("陌生人信息为 %v", stranger)
	}
}

// TestGroupMemberInfoToleratesPermissionError 说明成员权限关闭时仍然返回成功，
// 否则框架会丢弃 @ 消息段，群里 @ 机器人无法唤醒。
func TestGroupMemberInfoToleratesPermissionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":403,"msg":"没有权限"}`)
	}))
	defer server.Close()

	client, err := api.New(api.Config{BaseURL: server.URL + "/open-apis/v1", Token: "token", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("创建接口客户端失败: %v", err)
	}

	dispatcher := &Dispatcher{SelfID: 1001, API: client, Messages: identity.NewMemory()}

	data, err := dispatcher.Handle(context.Background(), "get_group_member_info", json.RawMessage(`{"group_id":20001,"user_id":10001}`))
	if err != nil {
		t.Fatalf("权限不足时应返回成功: %v", err)
	}
	if payload, ok := data.(map[string]any); !ok || len(payload) != 0 {
		t.Errorf("数据为空对象时框架才会补空昵称，实际为 %#v", data)
	}
}

func TestMetaActions(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	info, err := dispatcher.Handle(context.Background(), "get_login_info", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	login := info.(map[string]any)
	if login["user_id"] != int64(1001) || login["nickname"] != "机器人" {
		t.Errorf("登录信息为 %v", login)
	}

	status, err := dispatcher.Handle(context.Background(), "get_status", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if status.(map[string]any)["online"] != true {
		t.Errorf("状态为 %v", status)
	}

	version, err := dispatcher.Handle(context.Background(), "get_version_info", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if version.(map[string]any)["protocol_version"] != "v11" {
		t.Errorf("版本信息为 %v", version)
	}

	if _, err := dispatcher.Handle(context.Background(), ".handle_quick_operation_async", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("快速操作应当返回成功: %v", err)
	}
}

func TestSendSingleSegmentMessage(t *testing.T) {
	dispatcher, recorder := newDispatcher(t)

	// 个别框架会把单个消息段直接当作 message 传入。
	params := json.RawMessage(`{"group_id":20001,"message":{"type":"text","data":{"text":"单段"}}}`)
	if _, err := dispatcher.Handle(context.Background(), "send_group_msg", params); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	request := recorder.last(t)
	content := request.Content.(map[string]any)
	if text, _ := content["text"].(string); text != "单段" {
		t.Errorf("正文为 %q", content["text"])
	}
}

func TestMentionTextUsesNickname(t *testing.T) {
	nicknames := cache.NewNicknames(0)
	nicknames.Set("2272345", "洛雨辰")
	dispatcher := &Dispatcher{Nicknames: nicknames}

	if got := dispatcher.mentionText("@2272345 你好", []string{"2272345"}); got != "@洛雨辰 你好" {
		t.Errorf("正文为 %q", got)
	}

	// 没见过的用户保持标识，正文至少要能看出 @ 的是谁。
	if got := dispatcher.mentionText("@999 你好", []string{"999"}); got != "@999 你好" {
		t.Errorf("正文为 %q", got)
	}

	// 没有昵称缓存时不做替换。
	if got := (&Dispatcher{}).mentionText("@2272345 你好", []string{"2272345"}); got != "@2272345 你好" {
		t.Errorf("正文为 %q", got)
	}
}

// TestActionSuffixIsStripped 校验 _async 与 _rate_limited 两种调用后缀。
func TestActionSuffixIsStripped(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	for _, name := range []string{"get_status_async", "get_status_rate_limited", "get_status_async_rate_limited"} {
		data, err := dispatcher.Handle(context.Background(), name, nil)
		if err != nil {
			t.Fatalf("%s 处理失败: %v", name, err)
		}
		if data.(map[string]any)["online"] != true {
			t.Errorf("%s 返回 %v", name, data)
		}
	}

	// 未实现的接口带后缀时，错误里保留调用方写的名字，便于排查。
	_, err := dispatcher.Handle(context.Background(), "nonexistent_async", nil)
	if err == nil {
		t.Fatal("期望返回未实现错误")
	}
	var actionErr *obmodel.ActionError
	if !errors.As(err, &actionErr) || actionErr.Retcode != obmodel.RetcodeNotFound {
		t.Errorf("错误为 %v，期望 1404", err)
	}
	if !strings.Contains(actionErr.Wording, "nonexistent_async") {
		t.Errorf("错误措辞为 %q，应包含原始接口名", actionErr.Wording)
	}
}

// TestCanSendCapabilities 校验能力声明的返回值与配置一致。
func TestCanSendCapabilities(t *testing.T) {
	dispatcher, _ := newDispatcher(t)
	dispatcher.CanSendImage = true
	dispatcher.CanSendRecord = false

	image, err := dispatcher.Handle(context.Background(), "can_send_image", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if image.(map[string]any)["yes"] != true {
		t.Errorf("图片声明为 %v，期望真", image)
	}

	record, err := dispatcher.Handle(context.Background(), "can_send_record", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if record.(map[string]any)["yes"] != false {
		t.Errorf("语音声明为 %v，期望假", record)
	}
}

// TestGetFriendList 校验好友列表返回空数组而不是 null。
//
// 框架按数组解析响应，null 会让遍历直接失败，所以响应必须是 []。
func TestGetFriendList(t *testing.T) {
	dispatcher, _ := newDispatcher(t)

	data, err := dispatcher.Handle(context.Background(), "get_friend_list", nil)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("响应为 %s，期望 []", encoded)
	}
}
