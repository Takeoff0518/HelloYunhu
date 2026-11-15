// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// rawSegment 构造一个消息段，data 是 JSON 文本。
func rawSegment(t *testing.T, kind, data string) obmodel.InSegment {
	t.Helper()

	if !json.Valid([]byte(data)) {
		t.Fatalf("测试数据不是合法 JSON: %s", data)
	}
	return obmodel.InSegment{Type: kind, Data: json.RawMessage(data)}
}

// nodeSegment 构造一个 node 段。
func nodeSegment(t *testing.T, userID, nickname, content string) obmodel.InSegment {
	t.Helper()

	data := `{"user_id":` + userID + `,"nickname":` + nickname + `,"content":` + content + `}`
	return rawSegment(t, "node", data)
}

// textContent 构造只含一段文本的节点正文。
func textContent(text string) string {
	encoded, err := json.Marshal(text)
	if err != nil {
		panic(err)
	}
	return `[{"type":"text","data":{"text":` + string(encoded) + `}}]`
}

// joinForward 展开并拼成一段用于断言的文本。
func joinForward(t *testing.T, segments []obmodel.InSegment) string {
	t.Helper()

	parts, err := forwardText(segments)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	return strings.Join(parts, "\n")
}

func TestForwardTextBasic(t *testing.T) {
	segments := []obmodel.InSegment{
		nodeSegment(t, "10001", `"甲"`, textContent("早上好")),
		nodeSegment(t, "10002", `"乙"`, textContent("收到")),
	}

	got := joinForward(t, segments)
	for _, want := range []string{"1. 甲: 早上好", "2. 乙: 收到"} {
		if !strings.Contains(got, want) {
			t.Errorf("展开结果缺少 %q，实际为:\n%s", want, got)
		}
	}
}

// TestForwardNestedIndent 校验嵌套内容缩进一层并带层级标记。
func TestForwardNestedIndent(t *testing.T) {
	nested := `[{"type":"forward","data":{"content":[{"type":"node","data":` +
		string(nodeSegment(t, "10003", `"丙"`, textContent("在吗")).Data) + `}]}}]`
	segments := []obmodel.InSegment{
		nodeSegment(t, "10001", `"甲"`, textContent("你看这个")),
		nodeSegment(t, "10002", `"乙"`, nested),
	}

	got := joinForward(t, segments)
	if !strings.Contains(got, "  └ 1. 丙: 在吗") {
		t.Errorf("嵌套节点没有按预期缩进，实际为:\n%s", got)
	}
}

// TestForwardNicknameFallback 校验昵称为空时使用标识。
func TestForwardNicknameFallback(t *testing.T) {
	segments := []obmodel.InSegment{nodeSegment(t, "10001", `""`, textContent("你好"))}

	if got := joinForward(t, segments); !strings.Contains(got, "1. 10001: 你好") {
		t.Errorf("昵称为空时应使用标识，实际为:\n%s", got)
	}
}

// TestForwardEmptyIdentifier 校验只有标识的转发展开成一行说明。
func TestForwardEmptyIdentifier(t *testing.T) {
	segments := []obmodel.InSegment{rawSegment(t, "forward", `{"id":"abc123"}`)}

	if got := joinForward(t, segments); got != "[空的合并转发]" {
		t.Errorf("只有标识时应输出说明，实际为:\n%s", got)
	}
}

// TestForwardMixedOrder 校验文本与转发按原顺序拆分发送。
func TestForwardMixedOrder(t *testing.T) {
	segments := []obmodel.InSegment{
		rawSegment(t, "text", `{"text":"前面的文本"}`),
		nodeSegment(t, "10001", `"甲"`, textContent("转发内容")),
		rawSegment(t, "text", `{"text":"后面的文本"}`),
	}

	parts, err := Split(segments)
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("分段数量为 %d，期望 3", len(parts))
	}
	if !strings.Contains(parts[0].Text, "前面的文本") {
		t.Errorf("第一段为 %q", parts[0].Text)
	}
	if !strings.Contains(parts[1].Text, "转发内容") {
		t.Errorf("第二段为 %q", parts[1].Text)
	}
	if !strings.Contains(parts[2].Text, "后面的文本") {
		t.Errorf("第三段为 %q", parts[2].Text)
	}
}

// TestForwardChunking 校验长内容按字符上限分段并带编号标题。
func TestForwardChunking(t *testing.T) {
	long := strings.Repeat("字", 500)

	var segments []obmodel.InSegment
	for i := 0; i < 6; i++ {
		segments = append(segments, nodeSegment(t, "10001", `"甲"`, textContent(long)))
	}

	parts, err := forwardText(segments)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	if len(parts) < 2 {
		t.Fatalf("长内容应拆成多段，实际为 %d 段", len(parts))
	}

	for index, part := range parts {
		if got := utf8.RuneCountInString(part); got > forwardPartRunes {
			t.Errorf("第 %d 段有 %d 个字符，超过上限", index+1, got)
		}
		header := "[合并转发 " + strconv.Itoa(index+1) + "/" + strconv.Itoa(len(parts)) + "]"
		if !strings.HasPrefix(part, header) {
			t.Errorf("第 %d 段缺少标题 %q，开头为 %q", index+1, header, head(part))
		}
	}
}

// TestForwardHardSplit 校验单个超长节点被硬切。
func TestForwardHardSplit(t *testing.T) {
	segments := []obmodel.InSegment{
		nodeSegment(t, "10001", `"甲"`, textContent(strings.Repeat("长", 5000))),
	}

	parts, err := forwardText(segments)
	if err != nil {
		t.Fatalf("展开失败: %v", err)
	}
	if len(parts) < 3 {
		t.Fatalf("超长节点应被切成多段，实际为 %d 段", len(parts))
	}

	total := 0
	for index, part := range parts {
		total += utf8.RuneCountInString(part)
		if index == 0 {
			continue
		}
		body := part[strings.Index(part, "\n")+1:]
		if strings.Trim(body, "长") != "" {
			t.Errorf("第 %d 段正文不是原文片段: %q", index+1, head(body))
		}
	}
	if total < 5000 {
		t.Errorf("全部段落合计 %d 个字符，少于原文长度", total)
	}
}

// TestForwardDepthLimit 校验超过深度上限时停止展开。
func TestForwardDepthLimit(t *testing.T) {
	content := textContent("底层")
	for i := 0; i < forwardMaxDepth+2; i++ {
		inner := `[{"type":"node","data":{"user_id":1,"nickname":"甲","content":` + content + `}}]`
		content = `[{"type":"forward","data":{"content":` + inner + `}}]`
	}

	got := joinForward(t, []obmodel.InSegment{nodeSegment(t, "10001", `"甲"`, content)})
	if !strings.Contains(got, "嵌套过深") {
		t.Errorf("超过深度上限时应提示省略，实际为:\n%s", got)
	}
}

// TestForwardNodeLimit 校验节点总数上限。
func TestForwardNodeLimit(t *testing.T) {
	segments := make([]obmodel.InSegment, 0, forwardMaxNodes+10)
	for i := 0; i < forwardMaxNodes+10; i++ {
		segments = append(segments, nodeSegment(t, "10001", `"甲"`, textContent("内容")))
	}

	got := joinForward(t, segments)
	if !strings.Contains(got, "节点过多") {
		t.Errorf("超过节点上限时应提示省略，实际结尾为:\n%s", tail(got))
	}
}

// TestForwardCQStringContent 校验节点正文写成 CQ 码字符串时也能展开。
func TestForwardCQStringContent(t *testing.T) {
	segments := []obmodel.InSegment{
		nodeSegment(t, "10001", `"甲"`, `"[CQ:at,qq=123] 你好"`),
	}

	got := joinForward(t, segments)
	if !strings.Contains(got, "@123") || !strings.Contains(got, "你好") {
		t.Errorf("CQ 字符串正文展开异常，实际为:\n%s", got)
	}
}

// TestForwardMediaPlaceholders 校验节点内的媒体段使用占位文字。
func TestForwardMediaPlaceholders(t *testing.T) {
	content := `[{"type":"image","data":{"file":"a.png"}},` +
		`{"type":"file","data":{"file":"b.txt","name":"b.txt"}},` +
		`{"type":"record","data":{"file":"c.amr"}}]`
	segments := []obmodel.InSegment{nodeSegment(t, "10001", `"甲"`, content)}

	got := joinForward(t, segments)
	for _, want := range []string{"[图片]", "[文件: b.txt]", "[语音]"} {
		if !strings.Contains(got, want) {
			t.Errorf("展开结果缺少 %q，实际为:\n%s", want, got)
		}
	}
}

// head 返回字符串的开头，便于在断言失败时查看。
func head(text string) string {
	runes := []rune(text)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return text
}

// tail 返回字符串的结尾。
func tail(text string) string {
	runes := []rune(text)
	if len(runes) > 60 {
		return string(runes[len(runes)-60:])
	}
	return text
}
