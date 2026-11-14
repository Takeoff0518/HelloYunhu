// SPDX-License-Identifier: GPL-3.0-or-later

package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// openTemp 在临时目录里打开数据库。
func openTemp(t *testing.T) (*Store, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "data", "helloyunhu.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("关闭数据库失败: %v", err)
		}
	})

	return store, path
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("空路径应返回错误")
	}
}

func TestMessageIDIsStableAndIncrements(t *testing.T) {
	store, _ := openTemp(t)
	ctx := context.Background()

	first, err := store.MessageID(ctx, "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}
	if first != 1 {
		t.Errorf("第一个消息标识为 %d，期望 1", first)
	}

	again, err := store.MessageID(ctx, "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("重复记录失败: %v", err)
	}
	if again != first {
		t.Errorf("同一标识得到不同的整数: %d 与 %d", first, again)
	}

	second, err := store.MessageID(ctx, "ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}
	if second != 2 {
		t.Errorf("第二个消息标识为 %d，期望 2", second)
	}
}

func TestMsgIDLookup(t *testing.T) {
	store, _ := openTemp(t)
	ctx := context.Background()

	messageID, err := store.MessageID(ctx, "abc")
	if err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}

	msgID, ok, err := store.MsgID(ctx, messageID)
	if err != nil {
		t.Fatalf("反查失败: %v", err)
	}
	if !ok || msgID != "abc" {
		t.Errorf("反查结果为 %q, %v", msgID, ok)
	}

	if _, ok, err := store.MsgID(ctx, 9999); err != nil || ok {
		t.Errorf("未知标识的反查结果为 %v, %v", ok, err)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	store, path := openTemp(t)
	ctx := context.Background()

	messageID, err := store.MessageID(ctx, "persisted")
	if err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("关闭数据库失败: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("重新打开数据库失败: %v", err)
	}
	defer func() {
		_ = reopened.Close()
	}()

	msgID, ok, err := reopened.MsgID(ctx, messageID)
	if err != nil {
		t.Fatalf("反查失败: %v", err)
	}
	if !ok || msgID != "persisted" {
		t.Errorf("重启后反查结果为 %q, %v", msgID, ok)
	}

	again, err := reopened.MessageID(ctx, "persisted")
	if err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}
	if again != messageID {
		t.Errorf("重启后分配了新的整数: %d，期望 %d", again, messageID)
	}
}

func TestPruneRemovesOldRecords(t *testing.T) {
	store, _ := openTemp(t)
	ctx := context.Background()

	if _, err := store.MessageID(ctx, "old"); err != nil {
		t.Fatalf("记录消息标识失败: %v", err)
	}

	// 把写入时间改成很早以前，模拟超过保留时长的记录。
	if _, err := store.db.ExecContext(ctx, `UPDATE message_id SET created_at = 0`); err != nil {
		t.Fatalf("修改写入时间失败: %v", err)
	}

	removed, err := store.Prune(ctx, time.Hour)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if removed != 1 {
		t.Errorf("清理条数为 %d，期望 1", removed)
	}

	if _, ok, _ := store.MsgID(ctx, 1); ok {
		t.Error("清理后的记录仍然可以反查到")
	}
}

func TestPruneRejectsNonPositiveTTL(t *testing.T) {
	store, _ := openTemp(t)

	if _, err := store.Prune(context.Background(), 0); err == nil {
		t.Fatal("非正数的保留时长应返回错误")
	}
}
