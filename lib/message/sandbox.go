package message

import (
	"encoding/base64"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// maxSandboxImageBytes 是内联成 data URL 的上限: 再大就只留占位, 免得一个响应拖到几十 MB。
const maxSandboxImageBytes = 4 << 20

// SandboxMessage 是沙盒里捕获到的一条"本该发出去"的消息。
// 沙盒模式不真正投递、不进统计、也不进消息存档，只把内容回给管理台。
type SandboxMessage struct {
	Kind    string `json:"kind"`    // markdown / text / image / media
	Content string `json:"content"` // 可读正文；媒体为占位描述
	// URL 是图片的浏览器可加载地址(公网 URL 或 data URL); 为空表示前端只显示占位。
	URL string `json:"url,omitempty"`
}

// SandboxSink 收集沙盒消息，替代真实投递。
type SandboxSink struct {
	mu   sync.Mutex
	msgs []SandboxMessage
}

func NewSandboxSink() *SandboxSink { return &SandboxSink{} }

// Capture 记录一条消息。
func (s *SandboxSink) Capture(kind, content string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.msgs = append(s.msgs, SandboxMessage{Kind: kind, Content: content})
	s.mu.Unlock()
}

// CaptureImage 记录一条图片消息; url 非空时前端可直接渲染。
func (s *SandboxSink) CaptureImage(summary, url string) {
	if s == nil {
		return
	}
	if summary == "" {
		summary = "图片"
	}
	s.mu.Lock()
	s.msgs = append(s.msgs, SandboxMessage{Kind: "image", Content: "[" + summary + "]", URL: url})
	s.mu.Unlock()
}

// sandboxImageURL 把图片源归一化成浏览器能直接加载的地址(公网 URL 或 data URL)。
// 太大、读不到或不是媒体时返回空串, 前端就只显示占位。
func sandboxImageURL(src any) string {
	in, err := assets.Decode(src)
	if err != nil {
		return ""
	}
	if in.URL != "" {
		return in.URL
	}
	if len(in.Data) == 0 || len(in.Data) > maxSandboxImageBytes {
		return ""
	}
	mime := in.MimeType
	if mime == "" {
		mime = assets.ProbeMediaMime(in.Data)
	}
	if mime == "" {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(in.Data)
}

// CaptureMessage 按消息类型推断 kind 并取可读正文。
func (s *SandboxSink) CaptureMessage(msg *Message) {
	if s == nil || msg == nil {
		return
	}
	kind := "text"
	switch msg.Type {
	case constant.Markdown:
		kind = "markdown"
	case constant.Media:
		kind = "media"
	}
	content := bodyOf(msg)
	if content == "" {
		content = msg.MediaDesc
	}
	s.Capture(kind, content)
}

// Messages 返回已捕获消息的副本。
func (s *SandboxSink) Messages() []SandboxMessage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SandboxMessage, len(s.msgs))
	copy(out, s.msgs)
	return out
}
