// SPDX-License-Identifier: GPL-3.0-or-later

package message

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Takeoff0518/HelloYunhu/internal/bridge/cqcode"
	obmodel "github.com/Takeoff0518/HelloYunhu/internal/onebot/model"
)

// 合并转发的降级参数。
const (
	// forwardPartRunes 是每段文本的字符数上限，对应一条云湖消息。
	//
	// 云湖没有单条消息的长度上限，这个值只是为了让长转发读起来分段清晰，同时避免
	// 一条消息过长导致客户端滚动困难。
	forwardPartRunes = 2000
	// forwardMaxDepth 是嵌套层数上限，超过后不再展开。
	forwardMaxDepth = 8
	// forwardMaxNodes 是节点总数上限，防止异常构造消耗过多时间与消息条数。
	forwardMaxNodes = 500
	// forwardHeaderReserve 是分段标题预留的字符数。
	forwardHeaderReserve = 16
	// forwardUnavailable 是无法按标识读取转发内容时的占位行。
	forwardUnavailable = "[空的合并转发]"
)

// isForwardSegment 判断消息段是否为合并转发节点。
//
// OneBot 里合并转发有两种写法：消息段的 node（自带内容），以及 forward（可能只
// 带标识，也可能带内容）。
func isForwardSegment(segment obmodel.InSegment) bool {
	return segment.Type == "node" || segment.Type == "forward"
}

// forwardNode 是展开后的一个转发节点。
type forwardNode struct {
	userID   int64
	nickname string
	content  []obmodel.InSegment
	note     string
}

// forwardText 把一组转发节点展开成若干段文本，每段对应一条云湖消息。
//
// 展开结果按节点逐行排列，嵌套内容缩进一层。任何一段超过字符上限都会单独拆开，
// 多段时每段带「[合并转发 i/n]」标题，便于阅读顺序。
func forwardText(segments []obmodel.InSegment) ([]string, error) {
	nodes, err := parseForwardSegments(segments)
	if err != nil {
		return nil, err
	}

	renderer := &forwardRenderer{}
	renderer.render(nodes, 0, 0)

	if renderer.omitted {
		renderer.lines = append(renderer.lines, "…（节点过多，已省略）")
	}
	if len(renderer.lines) == 0 {
		return nil, nil
	}

	return chunkForward(renderer.lines), nil
}

// forwardRenderer 保存展开过程中的行与计数。
type forwardRenderer struct {
	lines   []string
	count   int
	omitted bool
}

// render 展开一层节点，depth 从 0 开始。
func (r *forwardRenderer) render(nodes []forwardNode, depth, start int) {
	for index, node := range nodes {
		// 只有标识没有内容的转发单独占一行，不编号。
		if node.note != "" && len(node.content) == 0 && node.nickname == "" && node.userID == 0 {
			r.lines = append(r.lines, strings.Repeat("  ", depth)+node.note)
			continue
		}

		if r.count >= forwardMaxNodes {
			r.omitted = true
			return
		}
		r.count++

		prefix := strings.Repeat("  ", depth)
		if depth > 0 {
			prefix += "└ "
		}

		title := node.nickname
		if title == "" {
			title = strconv.FormatInt(node.userID, 10)
		}

		inline, nested := renderForwardContent(node.content)
		line := fmt.Sprintf("%s%d. %s: %s", prefix, start+index+1, title, inline)
		if node.note != "" {
			line += node.note
		}
		r.lines = append(r.lines, strings.TrimRight(line, " "))

		if len(nested) == 0 {
			continue
		}
		if depth+1 >= forwardMaxDepth {
			r.lines = append(r.lines, prefix+"  …（嵌套过深，已省略）")
			continue
		}
		r.render(nested, depth+1, 0)
	}
}

// renderForwardContent 把节点内容渲染成一行文字，并返回其中的嵌套节点。
func renderForwardContent(segments []obmodel.InSegment) (string, []forwardNode) {
	var (
		builder strings.Builder
		nested  []forwardNode
	)

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var data obmodel.TextData
			if err := json.Unmarshal(segment.Data, &data); err == nil {
				builder.WriteString(strings.ReplaceAll(data.Text, "\n", " / "))
			}
		case "at":
			builder.WriteString(atPlaceholder(segment.Data))
		case "image":
			builder.WriteString("[图片]")
		case "file":
			builder.WriteString(filePlaceholder(segment.Data))
		case "video":
			builder.WriteString("[视频]")
		case "record":
			builder.WriteString("[语音]")
		case "face", "mface":
			builder.WriteString("[表情]")
		case "node":
			node, err := parseForwardNode(segment.Data)
			if err == nil {
				nested = append(nested, node)
			}
		case "forward":
			items, err := parseForwardPayload(segment.Data)
			if err == nil {
				nested = append(nested, items...)
			}
		default:
			builder.WriteString("[" + segment.Type + "]")
		}
	}

	return builder.String(), nested
}

// filePlaceholder 返回文件段的占位文字，带文件名时一并显示。
func filePlaceholder(raw json.RawMessage) string {
	var data struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Name == "" {
		return "[文件]"
	}
	return "[文件: " + data.Name + "]"
}

// atPlaceholder 返回 @ 段的占位文字。
//
// 节点里没有昵称信息，只能写标识；@全体成员 保留原样。
func atPlaceholder(raw json.RawMessage) string {
	var data struct {
		QQ json.RawMessage `json:"qq"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "@"
	}

	var text string
	if err := json.Unmarshal(data.QQ, &text); err == nil {
		if text == "all" {
			return "@全体成员"
		}
		return "@" + text
	}

	id, err := parseSegmentID(data.QQ)
	if err != nil || id == 0 {
		return "@"
	}
	return "@" + strconv.FormatInt(id, 10)
}

// parseForwardSegments 把消息段数组解析成节点列表。
func parseForwardSegments(segments []obmodel.InSegment) ([]forwardNode, error) {
	var nodes []forwardNode

	for index, segment := range segments {
		switch segment.Type {
		case "node":
			node, err := parseForwardNode(segment.Data)
			if err != nil {
				return nil, fmt.Errorf("第 %d 个转发节点解析失败: %w", index+1, err)
			}
			nodes = append(nodes, node)
		case "forward":
			items, err := parseForwardPayload(segment.Data)
			if err != nil {
				return nil, fmt.Errorf("第 %d 个转发段解析失败: %w", index+1, err)
			}
			nodes = append(nodes, items...)
		default:
			nodes = append(nodes, forwardNode{content: []obmodel.InSegment{segment}})
		}
	}

	return nodes, nil
}

// parseForwardPayload 解析 forward 段携带的内容。
//
// 只带标识的 forward 无法还原内容，展开成一行说明，保证发送路径继续工作。
func parseForwardPayload(raw json.RawMessage) ([]forwardNode, error) {
	var payload struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}

	if len(payload.Content) == 0 {
		return []forwardNode{{note: forwardUnavailable}}, nil
	}

	// 内容可能写成节点数组，也可能写成 CQ 码字符串。
	var segments []obmodel.InSegment
	if err := json.Unmarshal(payload.Content, &segments); err == nil {
		return parseForwardSegments(segments)
	}

	var text string
	if err := json.Unmarshal(payload.Content, &text); err == nil {
		return parseForwardSegments(cqSegments(text))
	}

	return nil, fmt.Errorf("无法识别的转发内容")
}

// parseForwardNode 解析一个 node 段的数据。
func parseForwardNode(raw json.RawMessage) (forwardNode, error) {
	var data struct {
		UserID   json.RawMessage `json:"user_id"`
		Nickname string          `json:"nickname"`
		Content  json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return forwardNode{}, err
	}

	node := forwardNode{nickname: strings.TrimSpace(data.Nickname)}
	if id, err := parseSegmentID(data.UserID); err == nil {
		node.userID = id
	}

	content, err := parseNodeContent(data.Content)
	if err != nil {
		return forwardNode{}, err
	}
	node.content = content

	return node, nil
}

// parseNodeContent 解析节点正文，支持消息段数组与 CQ 码字符串两种写法。
func parseNodeContent(raw json.RawMessage) ([]obmodel.InSegment, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var segments []obmodel.InSegment
	if err := json.Unmarshal(raw, &segments); err == nil {
		return segments, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return cqSegments(text), nil
	}

	return nil, fmt.Errorf("无法识别的节点正文")
}

// cqSegments 把 CQ 码字符串转换成消息段。
func cqSegments(text string) []obmodel.InSegment {
	parsed := cqcode.Parse(text)
	segments := make([]obmodel.InSegment, 0, len(parsed))

	for _, item := range parsed {
		payload := make(map[string]string, len(item.Data))
		for key, value := range item.Data {
			payload[key] = value
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		segments = append(segments, obmodel.InSegment{Type: item.Type, Data: encoded})
	}

	return segments
}

// parseSegmentID 解析可能是数字也可能是字符串的标识。
func parseSegmentID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strconv.ParseInt(text, 10, 64)
	}

	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.Int64()
	}

	return 0, fmt.Errorf("标识 %s 无法解析", raw)
}

// chunkForward 把展开后的行组合成若干段文本，每段不超过字符上限。
func chunkForward(lines []string) []string {
	capacity := forwardPartRunes - forwardHeaderReserve

	var (
		parts   []string
		builder strings.Builder
		size    int
	)

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		parts = append(parts, builder.String())
		builder.Reset()
		size = 0
	}

	for _, line := range lines {
		count := utf8.RuneCountInString(line)
		if count > capacity {
			flush()
			parts = append(parts, hardSplit(line, capacity)...)
			continue
		}

		if size > 0 && size+1+count > capacity {
			flush()
		}
		if size > 0 {
			builder.WriteString("\n")
			size++
		}
		builder.WriteString(line)
		size += count
	}
	flush()

	if len(parts) <= 1 {
		return parts
	}

	for index := range parts {
		parts[index] = fmt.Sprintf("[合并转发 %d/%d]\n%s", index+1, len(parts), parts[index])
	}
	return parts
}

// hardSplit 把一行超长文本按字符数切开。
func hardSplit(line string, capacity int) []string {
	var pieces []string

	runes := []rune(line)
	for start := 0; start < len(runes); start += capacity {
		end := start + capacity
		if end > len(runes) {
			end = len(runes)
		}
		pieces = append(pieces, string(runes[start:end]))
	}

	return pieces
}
