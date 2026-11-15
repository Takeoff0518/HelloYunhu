// SPDX-License-Identifier: GPL-3.0-or-later

package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Takeoff0518/HelloYunhu/internal/yunhu/model"
)

const testToken = "7c1f0a9e5b2d48c3a6f1e0d9b8c7a6f5"

// validEvent 是一条结构合法的事件。
const validEvent = `{"version":"1.0","header":{"eventId":"evt-1","eventType":"message.receive.normal","eventTime":1761936492340},"event":{}}`

// deliverFunc 记录投递结果。
type deliverFunc func(ctx context.Context, envelope model.Envelope) error

// serve 用给定投递函数与来源限制构造处理器。
func serve(t *testing.T, deliver deliverFunc, allowed []string, maxBody int64) http.Handler {
	t.Helper()

	server, err := New(Options{
		Token:        testToken,
		AllowedIPs:   allowed,
		Deliver:      deliver,
		MaxBodyBytes: maxBody,
	})
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	return server.Handler()
}

// post 向处理器发送一次请求。
func post(handler http.Handler, path, body, remoteAddr string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if remoteAddr != "" {
		request.RemoteAddr = remoteAddr
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestDeliversValidEvent(t *testing.T) {
	var got model.Envelope
	handler := serve(t, func(_ context.Context, envelope model.Envelope) error {
		got = envelope
		return nil
	}, nil, 0)

	recorder := post(handler, PathPrefix+testToken, validEvent, "192.0.2.10:1234")
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码为 %d", recorder.Code)
	}
	if got.Header.EventID != "evt-1" || got.Header.EventType != "message.receive.normal" {
		t.Errorf("投递的事件为 %+v", got.Header)
	}
}

func TestRejectsWrongToken(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return nil }, nil, 0)

	recorder := post(handler, PathPrefix+"wrong-token", validEvent, "")
	if recorder.Code != http.StatusNotFound {
		t.Errorf("状态码为 %d，期望 404", recorder.Code)
	}
}

func TestRejectsNonPost(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return nil }, nil, 0)

	request := httptest.NewRequest(http.MethodGet, PathPrefix+testToken, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("状态码为 %d，期望 405", recorder.Code)
	}
}

func TestRejectsBadPayload(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return nil }, nil, 0)

	if recorder := post(handler, PathPrefix+testToken, "{", ""); recorder.Code != http.StatusBadRequest {
		t.Errorf("非法 JSON 的状态码为 %d", recorder.Code)
	}
	if recorder := post(handler, PathPrefix+testToken, `{"header":{}}`, ""); recorder.Code != http.StatusBadRequest {
		t.Errorf("缺少 eventType 的状态码为 %d", recorder.Code)
	}
}

func TestRejectsOversizedBody(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return nil }, nil, 16)

	recorder := post(handler, PathPrefix+testToken, validEvent, "")
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("状态码为 %d，期望 413", recorder.Code)
	}
}

func TestBusyReturnsServiceUnavailable(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return ErrBusy }, nil, 0)

	recorder := post(handler, PathPrefix+testToken, validEvent, "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("状态码为 %d，期望 503", recorder.Code)
	}
}

func TestAllowedIPs(t *testing.T) {
	handler := serve(t, func(context.Context, model.Envelope) error { return nil }, []string{"203.0.113.0/24"}, 0)

	if recorder := post(handler, PathPrefix+testToken, validEvent, "198.51.100.7:1234"); recorder.Code != http.StatusForbidden {
		t.Errorf("不在网段内的状态码为 %d，期望 403", recorder.Code)
	}
	if recorder := post(handler, PathPrefix+testToken, validEvent, "203.0.113.7:1234"); recorder.Code != http.StatusOK {
		t.Errorf("网段内的状态码为 %d，期望 200", recorder.Code)
	}
}

// TestNewRejectsBadOptions 校验缺少令牌与非法网段被拒绝。
func TestNewRejectsBadOptions(t *testing.T) {
	if _, err := New(Options{Deliver: func(context.Context, model.Envelope) error { return nil }}); err == nil {
		t.Error("缺少令牌时应报错")
	}

	_, err := New(Options{
		Token:      testToken,
		AllowedIPs: []string{"not-a-cidr"},
		Deliver:    func(context.Context, model.Envelope) error { return nil },
	})
	if err == nil {
		t.Error("非法网段应报错")
	}
}
