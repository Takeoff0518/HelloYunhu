// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"encoding/json"
	"testing"
)

func TestIDValue(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int64
		wantErr bool
	}{
		{name: "数字", raw: `{"id":42}`, want: 42},
		{name: "字符串", raw: `{"id":"42"}`, want: 42},
		{name: "带空白的字符串", raw: `{"id":" 42 "}`, want: 42},
		{name: "非数字字符串", raw: `{"id":"abc"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var value struct {
				ID IDValue `json:"id"`
			}
			err := json.Unmarshal([]byte(tt.raw), &value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("期望返回错误")
				}
				return
			}
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if value.ID.Int64() != tt.want {
				t.Errorf("结果为 %d，期望 %d", value.ID.Int64(), tt.want)
			}
		})
	}
}
