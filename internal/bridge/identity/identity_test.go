// SPDX-License-Identifier: GPL-3.0-or-later

package identity

import (
	"context"
	"testing"
)

func TestMemoryAllocatesStableIDs(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()

	first, err := store.MessageID(ctx, "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("分配消息标识失败: %v", err)
	}
	if first != 1 {
		t.Errorf("第一个消息标识为 %d，期望 1", first)
	}

	again, err := store.MessageID(ctx, "0f1e2d3c4b5a69788796a5b4c3d2e1f0")
	if err != nil {
		t.Fatalf("重复分配失败: %v", err)
	}
	if again != first {
		t.Errorf("同一消息标识得到了不同的整数: %d 与 %d", first, again)
	}

	second, err := store.MessageID(ctx, "ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatalf("分配消息标识失败: %v", err)
	}
	if second != 2 {
		t.Errorf("第二个消息标识为 %d，期望 2", second)
	}
}

func TestMemoryLookup(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()

	id, err := store.MessageID(ctx, "abc")
	if err != nil {
		t.Fatalf("分配消息标识失败: %v", err)
	}

	msgID, ok, err := store.MsgID(ctx, id)
	if err != nil {
		t.Fatalf("反查失败: %v", err)
	}
	if !ok || msgID != "abc" {
		t.Errorf("反查结果为 %q, %v", msgID, ok)
	}

	if _, ok, err := store.MsgID(ctx, 9999); err != nil || ok {
		t.Errorf("未知消息标识的反查结果为 %v, %v", ok, err)
	}
}

func TestMemoryRejectsEmptyID(t *testing.T) {
	if _, err := NewMemory().MessageID(context.Background(), ""); err == nil {
		t.Fatal("空消息标识应返回错误")
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      int64
		wantExact bool
	}{
		{name: "普通数字", raw: "123456", want: 123456, wantExact: true},
		{name: "空字符串", raw: "", want: 0, wantExact: false},
		{name: "非数字", raw: "abc", wantExact: false},
		{name: "负数", raw: "-5", wantExact: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exact := ParseID(tt.raw)
			if exact != tt.wantExact {
				t.Errorf("精确解析标记为 %v，期望 %v", exact, tt.wantExact)
			}
			if tt.wantExact && got != tt.want {
				t.Errorf("结果为 %d，期望 %d", got, tt.want)
			}
			if !tt.wantExact && tt.raw != "" && got <= 0 {
				t.Errorf("哈希结果应当为正数，实际为 %d", got)
			}
		})
	}
}

func TestParseIDIsStable(t *testing.T) {
	first, _ := ParseID("not-a-number")
	second, _ := ParseID("not-a-number")
	if first != second {
		t.Errorf("同一输入的哈希结果不一致: %d 与 %d", first, second)
	}
}
