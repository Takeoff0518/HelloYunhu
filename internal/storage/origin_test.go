// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveOriginPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "helloyunhu.db")

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}

	ctx := context.Background()
	messageID, err := store.MessageID(ctx, "5a89c003122442e38fef935e03df5fd3")
	if err != nil {
		t.Fatalf("分配标识失败: %v", err)
	}
	if err := store.SaveOrigin(ctx, messageID, "695367192", "group"); err != nil {
		t.Fatalf("记录归属失败: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("关闭数据库失败: %v", err)
	}

	// 重启后仍要能读到会话信息，否则撤回旧消息会失效。
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("重新打开数据库失败: %v", err)
	}
	defer func() {
		_ = reopened.Close()
	}()

	chatID, chatType, ok, err := reopened.Origin(ctx, messageID)
	if err != nil {
		t.Fatalf("查询归属失败: %v", err)
	}
	if !ok || chatID != "695367192" || chatType != "group" {
		t.Errorf("归属为 %q %q，命中为 %v", chatID, chatType, ok)
	}
}

func TestOriginMissingReturnsFalse(t *testing.T) {
	store, _ := openTemp(t)

	if _, _, ok, err := store.Origin(context.Background(), 404); err != nil || ok {
		t.Errorf("未记录的标识返回 ok=%v，err=%v", ok, err)
	}
}

func TestOpenMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "helloyunhu.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	// 手工建一张旧结构的表，模拟升级前的数据库。
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("打开旧库失败: %v", err)
	}
	_, err = legacy.Exec(`CREATE TABLE message_id (
		message_id INTEGER PRIMARY KEY AUTOINCREMENT,
		msg_id     TEXT    NOT NULL UNIQUE,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("关闭旧库失败: %v", err)
	}

	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("升级后打开失败: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	ctx := context.Background()
	messageID, err := store.MessageID(ctx, "abc")
	if err != nil {
		t.Fatalf("分配标识失败: %v", err)
	}
	if err := store.SaveOrigin(ctx, messageID, "2272345", "bot"); err != nil {
		t.Fatalf("旧库升级后记录归属失败: %v", err)
	}
	if _, chatType, ok, _ := store.Origin(ctx, messageID); !ok || chatType != "bot" {
		t.Errorf("升级后的归属为 %q，命中为 %v", chatType, ok)
	}
}
