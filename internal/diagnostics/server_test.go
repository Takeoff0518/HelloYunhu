// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	started := time.Now().Add(-30 * time.Second)
	server := New(Options{
		Snapshot: func() Snapshot {
			return Snapshot{
				Version:        "v0.0.1",
				StartedAt:      started,
				UptimeSeconds:  30,
				BotID:          "11464576",
				Nickname:       "乐正绫TwinkLing",
				Connections:    []Connection{{Name: "astrbot", Online: true}},
				EventsReceived: 12,
				ActionsHandled: 3,
			}
		},
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Errorf("健康检查返回 %d %q", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态接口返回 %d", recorder.Code)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("解析状态失败: %v", err)
	}
	if snapshot.Version != "v0.0.1" || snapshot.BotID != "11464576" {
		t.Errorf("状态为 %+v", snapshot)
	}
	if len(snapshot.Connections) != 1 || !snapshot.Connections[0].Online {
		t.Errorf("连接状态为 %+v", snapshot.Connections)
	}
	if snapshot.EventsReceived != 12 || snapshot.ActionsHandled != 3 {
		t.Errorf("计数为 %+v", snapshot)
	}
}

func TestHandlerRejectsPost(t *testing.T) {
	server := New(Options{Snapshot: func() Snapshot { return Snapshot{} }})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/status", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("写操作返回 %d", recorder.Code)
	}
}
