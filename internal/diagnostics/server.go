// SPDX-License-Identifier: GPL-3.0-or-later

// Package diagnostics 提供只读的运行状态接口。
//
// 首个版本不提供管理界面，只暴露两个端点：健康检查返回固定文本，运行状态返回 JSON。
// 服务只读，不接受任何写操作，也不输出消息正文或个人数据。
package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Connection 是一条框架连接的在线状态。
type Connection struct {
	// Name 是连接名称。
	Name string `json:"name"`
	// Online 表示当前是否已建立连接。
	Online bool `json:"online"`
}

// Snapshot 是一次状态查询的结果。
type Snapshot struct {
	// Version 是构建版本。
	Version string `json:"version"`
	// StartedAt 是进程启动时间。
	StartedAt time.Time `json:"started_at"`
	// UptimeSeconds 是已运行的秒数。
	UptimeSeconds int64 `json:"uptime_seconds"`
	// BotID 是云湖机器人标识，启动查询成功前为空。
	BotID string `json:"bot_id,omitempty"`
	// Nickname 是机器人昵称。
	Nickname string `json:"nickname,omitempty"`
	// Connections 是各框架连接的状态。
	Connections []Connection `json:"connections"`
	// EventsReceived 是已处理的事件数量。
	EventsReceived int64 `json:"events_received"`
	// ActionsHandled 是已处理的框架调用数量。
	ActionsHandled int64 `json:"actions_handled"`
}

// Options 是诊断服务的设置。
type Options struct {
	// Listen 是监听地址，例如 127.0.0.1:9090。
	Listen string
	// Snapshot 返回当前状态。
	Snapshot func() Snapshot
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
}

// Server 是只读诊断服务。
type Server struct {
	listen   string
	snapshot func() Snapshot
	logger   *slog.Logger
}

// New 创建诊断服务。
func New(opt Options) *Server {
	logger := opt.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return &Server{listen: opt.Listen, snapshot: opt.Snapshot, logger: logger}
}

// Handler 返回只读端点。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.snapshot())
	})

	return mux
}

// Run 启动服务直到上下文结束。
func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              s.listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("诊断服务退出", "err", err)
		}
	}()

	s.logger.Info("诊断接口已启动", "listen", s.listen)

	select {
	case <-ctx.Done():
	case <-done:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	<-done
	return nil
}
