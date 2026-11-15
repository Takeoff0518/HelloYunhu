// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

var selftestUpgrader = websocket.Upgrader{}

// selftestConfig 生成指向给定地址的配置内容。
func selftestConfig(apiBase, subscribeURL, frameworkURL string) string {
	return `
bot:
  token: test-token
yunhu:
  api_base: ` + apiBase + `
  subscribe_url: ` + subscribeURL + `
onebot:
  - name: main
    url: ` + frameworkURL + `
`
}

// writeSelftestConfig 把配置写入临时文件。
func writeSelftestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
	return path
}

// wsAddr 把 httptest 地址改写为 WebSocket 地址。
func wsAddr(server *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + path
}

// statusOf 取指定名称的检查项状态。
func statusOf(results []CheckResult, name string) string {
	for _, result := range results {
		if result.Name == name {
			return result.Status
		}
	}
	return ""
}

func TestSelftestAllPassed(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"botId":"1001","nickname":"机器人"}}`))
	}))
	defer apiServer.Close()

	subscribeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("token"); got != "test-token" {
			t.Errorf("订阅令牌为 %q", got)
		}
		conn, err := selftestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer subscribeServer.Close()

	headers := make(chan string, 1)
	frameworkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("X-Self-ID")

		conn, err := selftestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer frameworkServer.Close()

	config := selftestConfig(apiServer.URL+"/open-apis/v1", wsAddr(subscribeServer, "/subscribe"), wsAddr(frameworkServer, "/ws"))

	results := Selftest(context.Background(), Options{ConfigPath: writeSelftestConfig(t, config)})

	for _, result := range results {
		if result.Status != StatusPassed {
			t.Errorf("检查项 %s 的状态为 %s，原因为 %s", result.Name, result.Status, result.Detail)
		}
	}

	select {
	case got := <-headers:
		if got != "1001" {
			t.Errorf("X-Self-ID 为 %q", got)
		}
	default:
		t.Error("框架没有收到握手")
	}
}

func TestSelftestConfigError(t *testing.T) {
	results := Selftest(context.Background(), Options{ConfigPath: filepath.Join(t.TempDir(), "absent.yaml")})

	if len(results) != 1 || results[0].Name != "配置检查" || results[0].Status != StatusFailed {
		t.Fatalf("自检结果为 %+v", results)
	}
}

func TestSelftestBotInfoFailed(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":401,"msg":"令牌无效"}`))
	}))
	defer apiServer.Close()

	subscribeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := selftestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer subscribeServer.Close()

	config := selftestConfig(apiServer.URL+"/open-apis/v1", wsAddr(subscribeServer, "/subscribe"), "ws://127.0.0.1:1/ws")

	results := Selftest(context.Background(), Options{ConfigPath: writeSelftestConfig(t, config)})

	if got := statusOf(results, "机器人信息"); got != StatusFailed {
		t.Errorf("机器人信息检查的状态为 %q", got)
	}
	if got := statusOf(results, "框架握手"); got != StatusSkipped {
		t.Errorf("框架握手的状态为 %q，期望跳过", got)
	}
	if got := statusOf(results, "订阅连接"); got != StatusPassed {
		t.Errorf("订阅连接的状态为 %q", got)
	}
}

func TestSelftestSubscribeFailed(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"botId":"1001","nickname":"机器人"}}`))
	}))
	defer apiServer.Close()

	subscribeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer subscribeServer.Close()

	frameworkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := selftestUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer frameworkServer.Close()

	config := selftestConfig(apiServer.URL+"/open-apis/v1", wsAddr(subscribeServer, "/subscribe"), wsAddr(frameworkServer, "/ws"))

	results := Selftest(context.Background(), Options{ConfigPath: writeSelftestConfig(t, config)})

	if got := statusOf(results, "订阅连接"); got != StatusFailed {
		t.Errorf("订阅连接的状态为 %q，期望失败", got)
	}
	if got := statusOf(results, "框架握手 main"); got != StatusPassed {
		t.Errorf("框架握手的状态为 %q", got)
	}
}
