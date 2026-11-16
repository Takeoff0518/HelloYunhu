// SPDX-License-Identifier: GPL-3.0-or-later

// Package proxy 对客户端提供可下载的云湖媒体与头像。
//
// 云湖的媒体地址带来源校验，客户端直接下载会得到 403。这个服务以自己的地址对外，
// 取图时附带来源头，把内容转发给客户端。
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Takeoff0518/HelloYunhu/internal/proxypath"
)

// Options 是代理服务的设置。
type Options struct {
	// Addr 是监听地址。
	Addr string
	// Avatar 是头像处理器，为空时不提供头像路径。
	Avatar http.Handler
	// Media 是媒体处理器，为空时不提供媒体路径。
	Media http.Handler
	// Logger 为空时丢弃日志。
	Logger *slog.Logger
}

// Run 启动代理服务直到上下文结束。
func Run(ctx context.Context, opt Options) error {
	logger := opt.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	if opt.Avatar == nil && opt.Media == nil {
		return errors.New("代理服务缺少可用的处理路径")
	}

	mux := http.NewServeMux()
	if opt.Avatar != nil {
		mux.Handle(proxypath.Avatar, opt.Avatar)
	}
	if opt.Media != nil {
		mux.Handle(proxypath.Media, opt.Media)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	server := &http.Server{
		Addr:              opt.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		// 媒体可能很大，不设写超时，避免传输大文件时被强制中断。
		WriteTimeout: 0,
	}

	done := make(chan struct{})
	var serveErr error
	go func() {
		defer close(done)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
			logger.Error("代理服务退出", "listen", opt.Addr, "err", err)
		}
	}()

	paths := make([]string, 0, 2)
	if opt.Avatar != nil {
		paths = append(paths, proxypath.Avatar+"<标识>")
	}
	if opt.Media != nil {
		paths = append(paths, proxypath.Media+"<token>")
	}
	logger.Info("代理服务已启动", "listen", opt.Addr, "paths", paths)

	select {
	case <-ctx.Done():
	case <-done:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	// 端口被占用时把错误交回启动流程，避免留下一个静默失效的服务。
	<-done
	return serveErr
}
