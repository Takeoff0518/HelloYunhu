// SPDX-License-Identifier: GPL-3.0-or-later

// Package storage 提供 SQLite 持久化。
//
// 只保存标识映射与去重记录，不保存消息正文，避免引入隐私问题。驱动使用纯 Go
// 实现，不依赖 cgo，便于交叉编译到各平台。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/identity"
)

// Store 是基于 SQLite 的存储。
type Store struct {
	db *sql.DB
}

// Store 实现消息标识映射接口。
var _ identity.MessageStore = (*Store)(nil)

// schema 是当前的表结构。语句逐条执行，重复执行不会报错。
var schema = []string{
	`CREATE TABLE IF NOT EXISTS message_id (
		message_id INTEGER PRIMARY KEY AUTOINCREMENT,
		msg_id     TEXT    NOT NULL UNIQUE,
		created_at INTEGER NOT NULL,
		chat_id    TEXT    NOT NULL DEFAULT '',
		chat_type  TEXT    NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_message_id_created_at ON message_id (created_at)`,
}

// migrations 是旧库升级用的语句。每一条都在对应的列缺失时执行一次。
var migrations = []struct {
	column    string
	statement string
}{
	{column: "chat_id", statement: `ALTER TABLE message_id ADD COLUMN chat_id TEXT NOT NULL DEFAULT ''`},
	{column: "chat_type", statement: `ALTER TABLE message_id ADD COLUMN chat_type TEXT NOT NULL DEFAULT ''`},
}

// Open 打开数据库并初始化表结构。文件所在目录不存在时会自动创建。
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("数据库路径不能为空")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析数据库路径 %s: %w", path, err)
	}
	if dir := filepath.Dir(absolute); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录 %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", dsn(absolute))
	if err != nil {
		return nil, fmt.Errorf("打开数据库 %s: %w", absolute, err)
	}

	// SQLite 同一时刻只允许一个写者，单连接可以让并发调用排队而不是报错。
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// dsn 生成带连接参数的 DSN。
//
// busy_timeout 让锁冲突时等待而不是立即失败，WAL 让读写可以同时进行。
func dsn(absolute string) string {
	return "file:" + filepath.ToSlash(absolute) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// migrate 建表并升级旧库。表结构变更时在这里追加语句，不做破坏性修改。
func (s *Store) migrate(ctx context.Context) error {
	for _, statement := range schema {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化数据库表结构: %w", err)
		}
	}

	columns, err := s.columns(ctx)
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if columns[migration.column] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, migration.statement); err != nil {
			return fmt.Errorf("升级数据库表结构，新增 %s 列: %w", migration.column, err)
		}
	}

	return nil
}

// columns 返回 message_id 表已有的列。
func (s *Store) columns(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(message_id)`)
	if err != nil {
		return nil, fmt.Errorf("读取表结构: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			index    int
			name     string
			dataType string
			notNull  int
			defaultV any
			primary  int
		)
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultV, &primary); err != nil {
			return nil, fmt.Errorf("读取列信息: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历列信息: %w", err)
	}

	return columns, nil
}

// constSelectMessageID 是按云湖标识查询的语句。
const constSelectMessageID = `SELECT message_id FROM message_id WHERE msg_id = ?`

// MessageID 返回消息标识对应的整数，没有记录时分配一个新的。
//
// 先查询再写入：命中时完全不写库，避免自增序列被无效写入消耗。写入使用
// DO NOTHING，若同时有别的调用者抢先写入，重新查询即可拿到同一个整数。
func (s *Store) MessageID(ctx context.Context, msgID string) (int64, error) {
	if msgID == "" {
		return 0, errors.New("消息标识不能为空")
	}

	var messageID int64
	switch err := s.db.QueryRowContext(ctx, constSelectMessageID, msgID).Scan(&messageID); {
	case err == nil:
		return messageID, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("查询消息标识: %w", err)
	}

	const insert = `
		INSERT INTO message_id (msg_id, created_at) VALUES (?, ?)
		ON CONFLICT (msg_id) DO NOTHING
		RETURNING message_id`

	switch err := s.db.QueryRowContext(ctx, insert, msgID, time.Now().Unix()).Scan(&messageID); {
	case err == nil:
		return messageID, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := s.db.QueryRowContext(ctx, constSelectMessageID, msgID).Scan(&messageID); err != nil {
			return 0, fmt.Errorf("查询消息标识: %w", err)
		}
		return messageID, nil
	default:
		return 0, fmt.Errorf("记录消息标识: %w", err)
	}
}

// MsgID 按整数反查云湖消息标识，没有记录时第二个返回值为 false。
func (s *Store) MsgID(ctx context.Context, messageID int64) (string, bool, error) {
	const statement = `SELECT msg_id FROM message_id WHERE message_id = ?`

	var msgID string
	switch err := s.db.QueryRowContext(ctx, statement, messageID).Scan(&msgID); {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("查询消息标识: %w", err)
	default:
		return msgID, true, nil
	}
}

// SaveOrigin 记录消息的会话归属，撤回时使用。
//
// 记录一定存在：分配的标识来自 MessageID，那条记录已经写入。找不到时说明调用顺序
// 有问题，这里返回错误而不是静默插入一行残缺记录。
func (s *Store) SaveOrigin(ctx context.Context, messageID int64, chatID, chatType string) error {
	if messageID <= 0 || chatID == "" {
		return nil
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE message_id SET chat_id = ?, chat_type = ? WHERE message_id = ?`,
		chatID, chatType, messageID)
	if err != nil {
		return fmt.Errorf("记录消息归属: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取更新条数: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("记录消息归属: 消息 %d 不存在", messageID)
	}

	return nil
}

// Origin 返回消息的会话归属。
func (s *Store) Origin(ctx context.Context, messageID int64) (string, string, bool, error) {
	if messageID <= 0 {
		return "", "", false, nil
	}

	const statement = `SELECT chat_id, chat_type FROM message_id WHERE message_id = ?`

	var chatID, chatType string
	switch err := s.db.QueryRowContext(ctx, statement, messageID).Scan(&chatID, &chatType); {
	case errors.Is(err, sql.ErrNoRows):
		return "", "", false, nil
	case err != nil:
		return "", "", false, fmt.Errorf("查询消息归属: %w", err)
	case chatID == "":
		return "", "", false, nil
	default:
		return chatID, chatType, true, nil
	}
}

// Prune 删除早于给定时长的映射记录，返回删除的条数。
func (s *Store) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	if olderThan <= 0 {
		return 0, fmt.Errorf("保留时长必须大于 0，当前为 %s", olderThan)
	}

	cutoff := time.Now().Add(-olderThan).Unix()

	result, err := s.db.ExecContext(ctx, `DELETE FROM message_id WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理消息标识: %w", err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("读取清理条数: %w", err)
	}

	return removed, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
