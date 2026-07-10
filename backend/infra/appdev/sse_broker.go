/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appdev

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	"github.com/coze-dev/coze-studio/backend/bizpkg/llm/modelbuilder"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"

	"github.com/cloudwego/eino/schema"
)

type ChatBroker struct {
	mu          sync.Mutex
	subscribers map[string]map[chan *appdevapp.ChatEvent]struct{}
	history     map[string][]*appdevapp.ChatMessage
	status      map[string]*appdevapp.ChatStatus
	cancels     map[string]context.CancelFunc
	persistence ChatPersistence
}

type appDevSourceResult struct {
	source           string
	generatedByModel bool
	prototypeImages  int
}

var (
	appDevModelGenerationTimeout     = 90 * time.Second
	appDevChatLeaseHeartbeatInterval = time.Minute
	appDevChatLeaseHeartbeatTimeout  = 10 * time.Second
)

const (
	appDevMaxPrototypeImageParts = 3
	appDevMaxPrototypeImageBytes = 4 * 1024 * 1024
)

func NewChatBroker() *ChatBroker {
	return NewChatBrokerWithPersistence(nil)
}

func NewChatBrokerWithPersistence(persistence ChatPersistence) *ChatBroker {
	return &ChatBroker{
		subscribers: map[string]map[chan *appdevapp.ChatEvent]struct{}{},
		history:     map[string][]*appdevapp.ChatMessage{},
		status:      map[string]*appdevapp.ChatStatus{},
		cancels:     map[string]context.CancelFunc{},
		persistence: persistence,
	}
}

func (b *ChatBroker) Send(ctx context.Context, req *appdevapp.ChatManagerRequest) (*appdevapp.ChatSession, error) {
	key := chatKey(req)
	session := &appdevapp.ChatSession{
		SessionID: fmt.Sprintf("session_%d", time.Now().UnixNano()),
		RequestID: fmt.Sprintf("request_%d", time.Now().UnixNano()),
		Running:   true,
	}

	generationCtx, cancel := context.WithCancel(context.Background())

	b.mu.Lock()
	if status := b.status[key]; status != nil && status.Running {
		b.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("appdev task is already running")
	}
	userMessage := &appdevapp.ChatMessage{
		ID:          "user_" + session.RequestID,
		Type:        "user",
		Role:        "user",
		Content:     req.Message,
		Attachments: req.Attachments,
		CreatedAt:   time.Now().UTC(),
	}
	b.history[key] = append(b.history[key], userMessage)
	b.status[key] = &appdevapp.ChatStatus{
		Running:   true,
		SessionID: session.SessionID,
		RequestID: session.RequestID,
	}
	b.cancels[key] = cancel
	b.mu.Unlock()
	if b.persistence != nil {
		if err := b.persistence.BeginSession(ctx, req, session, userMessage); err != nil {
			cancel()
			b.mu.Lock()
			b.status[key] = &appdevapp.ChatStatus{Running: false}
			delete(b.cancels, key)
			if messages := b.history[key]; len(messages) > 0 && messages[len(messages)-1].ID == userMessage.ID {
				b.history[key] = messages[:len(messages)-1]
			}
			b.mu.Unlock()
			return nil, err
		}
	}

	go b.renewChatLease(generationCtx, req, session.RequestID)
	go func() {
		defer cancel()
		b.emitGeneratedResponse(generationCtx, key, session, req)
	}()

	return session, nil
}

func (b *ChatBroker) renewChatLease(ctx context.Context, req *appdevapp.ChatManagerRequest, requestID string) {
	if b.persistence == nil {
		return
	}
	interval := appDevChatLeaseHeartbeatInterval
	if interval <= 0 {
		interval = time.Minute
	}
	timeout := appDevChatLeaseHeartbeatTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			touchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			_ = b.persistence.TouchSession(touchCtx, req, requestID)
			cancel()
		}
	}
}

func (b *ChatBroker) Subscribe(ctx context.Context, req *appdevapp.ChatManagerRequest) (<-chan *appdevapp.ChatEvent, func(), error) {
	key := chatKey(req)
	ch := make(chan *appdevapp.ChatEvent, 32)

	b.mu.Lock()
	if b.subscribers[key] == nil {
		b.subscribers[key] = map[chan *appdevapp.ChatEvent]struct{}{}
	}
	b.subscribers[key][ch] = struct{}{}
	b.mu.Unlock()

	cleanup := func() {
		b.mu.Lock()
		if subscribers := b.subscribers[key]; subscribers != nil {
			delete(subscribers, ch)
		}
		b.mu.Unlock()
	}

	return ch, cleanup, nil
}

func (b *ChatBroker) Cancel(ctx context.Context, req *appdevapp.ChatManagerRequest) error {
	key := chatKey(req)

	b.mu.Lock()
	status := b.status[key]
	if status == nil || !status.Running {
		b.mu.Unlock()
		if b.persistence == nil {
			return nil
		}
		persisted, err := b.persistence.Status(ctx, req)
		if err != nil {
			return err
		}
		if persisted == nil || !persisted.Running {
			return nil
		}
		message := cancelledHistoryMessage(persisted.RequestID)
		persisted.Running = false
		if err := b.persistence.FinishSession(ctx, req, persisted, message); err != nil {
			return err
		}
		b.mu.Lock()
		b.history[key] = append(b.history[key], message)
		b.status[key] = persisted
		b.mu.Unlock()
		b.broadcast(key, &appdevapp.ChatEvent{
			Event: "prompt_end",
			Data:  map[string]any{"status": "cancelled"},
		})
		return nil
	}
	if cancel := b.cancels[key]; cancel != nil {
		cancel()
		delete(b.cancels, key)
	}
	cancelledMessage := cancelledHistoryMessage(status.RequestID)
	cancelledStatus := &appdevapp.ChatStatus{
		Running:   false,
		SessionID: status.SessionID,
		RequestID: status.RequestID,
	}
	b.history[key] = append(b.history[key], cancelledMessage)
	b.status[key] = cancelledStatus
	b.mu.Unlock()
	if b.persistence != nil {
		if err := b.persistence.FinishSession(ctx, req, cancelledStatus, cancelledMessage); err != nil {
			return err
		}
	}

	b.broadcast(key, &appdevapp.ChatEvent{
		Event: "prompt_end",
		Data: map[string]any{
			"status": "cancelled",
		},
	})

	return nil
}

func cancelledHistoryMessage(requestID string) *appdevapp.ChatMessage {
	return &appdevapp.ChatMessage{
		ID:        "cancelled_" + requestID,
		Type:      "assistant",
		Role:      "assistant",
		Content:   "任务已取消，未修改项目文件。",
		CreatedAt: time.Now().UTC(),
	}
}

func (b *ChatBroker) History(ctx context.Context, req *appdevapp.ChatManagerRequest) ([]*appdevapp.ChatMessage, error) {
	if b.persistence != nil {
		return b.persistence.History(ctx, req)
	}
	key := chatKey(req)

	b.mu.Lock()
	defer b.mu.Unlock()

	messages := make([]*appdevapp.ChatMessage, len(b.history[key]))
	copy(messages, b.history[key])
	return messages, nil
}

func (b *ChatBroker) Status(ctx context.Context, req *appdevapp.ChatManagerRequest) (*appdevapp.ChatStatus, error) {
	if b.persistence != nil {
		return b.persistence.Status(ctx, req)
	}
	key := chatKey(req)

	b.mu.Lock()
	defer b.mu.Unlock()

	if status := b.status[key]; status != nil {
		return status, nil
	}

	return &appdevapp.ChatStatus{Running: false}, nil
}

func (b *ChatBroker) emitGeneratedResponse(ctx context.Context, key string, session *appdevapp.ChatSession, req *appdevapp.ChatManagerRequest) {
	events := []*appdevapp.ChatEvent{
		{Event: "prompt_start", Data: map[string]any{"sessionId": session.SessionID, "requestId": session.RequestID}},
		{Event: "agent_thought_chunk", Data: map[string]any{"id": "thinking_" + session.RequestID, "content": "正在理解网页应用需求并准备开发计划..."}},
		{Event: "tool_call", Data: map[string]any{"id": "tool_" + session.RequestID, "title": "生成网页应用代码", "summary": "根据需求更新 src/App.tsx"}},
	}

	for _, event := range events {
		if ctx.Err() != nil || !b.isRunning(key, session.RequestID) {
			b.clearCancel(key)
			return
		}
		time.Sleep(180 * time.Millisecond)
		b.broadcast(key, event)
	}

	if ctx.Err() != nil || !b.isRunning(key, session.RequestID) {
		b.clearCancel(key)
		return
	}

	generatedPath := "src/App.tsx"
	source, generatedByModel, prototypeImages := generateAppSource(ctx, req)
	if !b.canCommitGeneratedSource(ctx, key, session.RequestID, req) {
		b.clearCancel(key)
		return
	}
	if event := prototypeImageProgressEvent(
		session.RequestID,
		countPrototypeImageAttachments(req.Attachments),
		prototypeImages,
	); event != nil {
		b.broadcast(key, event)
	}
	var writeErr error
	if req.SaveGeneratedFile != nil {
		writeErr = req.SaveGeneratedFile(ctx, generatedPath, source)
	} else {
		writeErr = writeProjectFile(req.ProjectDir, generatedPath, source)
	}
	if writeErr != nil {
		b.broadcast(key, &appdevapp.ChatEvent{
			Event: "error",
			Data: map[string]any{
				"message": "写入网页应用代码失败，请稍后重试或检查项目文件权限",
			},
		})
		b.mu.Lock()
		failedStatus := &appdevapp.ChatStatus{
			Running:   false,
			SessionID: session.SessionID,
			RequestID: session.RequestID,
		}
		b.status[key] = failedStatus
		delete(b.cancels, key)
		b.mu.Unlock()
		if b.persistence != nil {
			_ = b.persistence.FinishSession(context.Background(), req, failedStatus, nil)
		}
		return
	}

	if ctx.Err() != nil || !b.isRunning(key, session.RequestID) {
		b.clearCancel(key)
		return
	}

	b.broadcast(key, &appdevapp.ChatEvent{
		Event: "tool_call_update",
		Data: map[string]any{
			"id":      "tool_" + session.RequestID,
			"title":   "写入文件",
			"summary": appDevWriteSummary(generatedPath, generatedByModel),
		},
	})
	b.broadcast(key, &appdevapp.ChatEvent{
		Event: "agent_message_chunk",
		Data: map[string]any{
			"id": "assistant_" + session.RequestID,
			"content": fmt.Sprintf(
				"已根据你的需求更新 `%s`。\n\n可以在文件树中打开文件查看代码，或启动开发环境预览页面效果。",
				generatedPath,
			),
		},
	})
	b.broadcast(key, &appdevapp.ChatEvent{
		Event: "prompt_end",
		Data: map[string]any{
			"status": "success",
			"files":  []string{generatedPath},
		},
	})

	assistantMessage := &appdevapp.ChatMessage{
		ID:        "assistant_" + session.RequestID,
		Type:      "assistant",
		Role:      "assistant",
		Content:   fmt.Sprintf("已根据你的需求更新 `%s`，可以继续预览或提出修改。", generatedPath),
		CreatedAt: time.Now().UTC(),
	}
	completedStatus := &appdevapp.ChatStatus{
		Running:   false,
		SessionID: session.SessionID,
		RequestID: session.RequestID,
	}
	b.mu.Lock()
	b.history[key] = append(b.history[key], assistantMessage)
	b.status[key] = completedStatus
	delete(b.cancels, key)
	b.mu.Unlock()
	if b.persistence != nil {
		_ = b.persistence.FinishSession(context.Background(), req, completedStatus, assistantMessage)
	}
}

func canCommitGeneratedSource(ctx context.Context, running bool) bool {
	return running && ctx.Err() == nil
}

func (b *ChatBroker) canCommitGeneratedSource(
	ctx context.Context,
	key string,
	requestID string,
	req *appdevapp.ChatManagerRequest,
) bool {
	if !canCommitGeneratedSource(ctx, b.isRunning(key, requestID)) {
		return false
	}
	if b.persistence == nil {
		return true
	}
	status, err := b.persistence.Status(ctx, req)
	return err == nil && status != nil && status.Running && status.RequestID == requestID
}

func writeProjectFile(projectDir string, filePath string, source string) error {
	normalized, err := domainappdev.NormalizeRelativePath(filePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(projectDir) == "" {
		return fmt.Errorf("project directory is empty")
	}

	fullPath := filepath.Join(projectDir, filepath.FromSlash(normalized))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(source), 0o644)
}

func generateAppSource(ctx context.Context, req *appdevapp.ChatManagerRequest) (string, bool, int) {
	fallback := generatedAppSource(req.Message)
	modelID, err := strconv.ParseInt(req.ModelID, 10, 64)
	if err != nil {
		return fallback, false, 0
	}

	return generateAppSourceWithPrototypeStatusTimeout(
		ctx,
		fallback,
		appDevModelGenerationTimeout,
		func(generationCtx context.Context) (string, bool, int) {
			return generateAppSourceByModel(generationCtx, req, modelID)
		},
	)
}

func generateAppSourceWithTimeout(
	ctx context.Context,
	fallback string,
	timeout time.Duration,
	generator func(context.Context) (string, bool),
) (string, bool) {
	source, generatedByModel, _ := generateAppSourceWithPrototypeStatusTimeout(
		ctx,
		fallback,
		timeout,
		func(generationCtx context.Context) (string, bool, int) {
			source, generatedByModel := generator(generationCtx)
			return source, generatedByModel, 0
		},
	)
	return source, generatedByModel
}

func generateAppSourceWithPrototypeStatusTimeout(
	ctx context.Context,
	fallback string,
	timeout time.Duration,
	generator func(context.Context) (string, bool, int),
) (string, bool, int) {
	if timeout <= 0 {
		timeout = appDevModelGenerationTimeout
	}

	generationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan appDevSourceResult, 1)
	go func() {
		source, generatedByModel, prototypeImages := generator(generationCtx)
		resultCh <- appDevSourceResult{
			source:           source,
			generatedByModel: generatedByModel,
			prototypeImages:  prototypeImages,
		}
	}()

	select {
	case result := <-resultCh:
		if !result.generatedByModel {
			return fallback, false, 0
		}
		return result.source, true, result.prototypeImages
	case <-generationCtx.Done():
		return fallback, false, 0
	}
}

func generateAppSourceByModel(ctx context.Context, req *appdevapp.ChatManagerRequest, modelID int64) (string, bool, int) {
	chatModel, info, err := modelbuilder.BuildModelByID(ctx, modelID, &modelbuilder.LLMParams{
		EnableThinking: ptr.Of(false),
	})
	if err != nil {
		return "", false, 0
	}

	enablePrototypeImages := false
	if info != nil && info.Model != nil && info.EnableBase64URL && info.Capability != nil {
		enablePrototypeImages = info.Capability.GetSupportMultiModal() && info.Capability.GetImageUnderstanding()
	}

	userMessage, prototypeImages := buildAppDevUserMessageWithPrototypeImages(req, enablePrototypeImages)
	response, err := chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(appDevCodeSystemPrompt()),
		userMessage,
	})
	if err != nil || response == nil {
		return "", false, 0
	}

	source := extractTSXSource(response.Content)
	if !isSafeAppSource(source) {
		return "", false, 0
	}

	return source, true, prototypeImages
}

func buildAppDevUserMessage(req *appdevapp.ChatManagerRequest, enablePrototypeImages bool) *schema.Message {
	message, _ := buildAppDevUserMessageWithPrototypeImages(req, enablePrototypeImages)
	return message
}

func buildAppDevUserMessageWithPrototypeImages(
	req *appdevapp.ChatManagerRequest,
	enablePrototypeImages bool,
) (*schema.Message, int) {
	prompt := buildAppDevUserPrompt(req)
	if !enablePrototypeImages {
		return schema.UserMessage(prompt), 0
	}

	parts := []schema.ChatMessagePart{
		{
			Type: schema.ChatMessagePartTypeText,
			Text: prompt,
		},
	}

	for _, attachment := range req.Attachments {
		part, ok := buildAppDevPrototypeImagePart(req.ProjectDir, attachment)
		if !ok {
			continue
		}
		parts = append(parts, part)
		if len(parts)-1 >= appDevMaxPrototypeImageParts {
			break
		}
	}

	if len(parts) == 1 {
		return schema.UserMessage(prompt), 0
	}

	return &schema.Message{
		Role:         schema.User,
		MultiContent: parts,
	}, len(parts) - 1
}

func countPrototypeImageAttachments(attachments []appdevapp.ChatAttachment) int {
	count := 0
	for _, attachment := range attachments {
		if attachment.Type == "prototype_image" {
			count++
		}
	}
	return count
}

func prototypeImageProgressEvent(requestID string, requested int, used int) *appdevapp.ChatEvent {
	if requested <= 0 {
		return nil
	}

	summary := "本次原型图未作为视觉输入，已按文字描述和附件路径降级参考。"
	if used > 0 {
		summary = fmt.Sprintf("已将 %d 张原型图作为视觉上下文发送给模型。", used)
		if used < requested {
			summary = fmt.Sprintf(
				"已将 %d 张原型图作为视觉上下文发送给模型；另 %d 张未读取，已按文字描述和附件路径降级参考。",
				used,
				requested-used,
			)
		}
	}

	return &appdevapp.ChatEvent{
		Event: "tool_call_update",
		Data: map[string]any{
			"id":      "tool_prototype_" + requestID,
			"title":   "读取原型图",
			"summary": summary,
		},
	}
}

func buildAppDevPrototypeImagePart(projectDir string, attachment appdevapp.ChatAttachment) (schema.ChatMessagePart, bool) {
	if attachment.Type != "prototype_image" {
		return schema.ChatMessagePart{}, false
	}
	if !strings.HasPrefix(attachment.MIMEType, "image/") {
		return schema.ChatMessagePart{}, false
	}
	if strings.TrimSpace(projectDir) == "" {
		return schema.ChatMessagePart{}, false
	}

	filePath, err := domainappdev.NormalizeRelativePath(attachment.Path)
	if err != nil {
		return schema.ChatMessagePart{}, false
	}
	if !strings.HasPrefix(filePath, "src/assets/uploads/") {
		return schema.ChatMessagePart{}, false
	}

	fullPath := filepath.Join(projectDir, filepath.FromSlash(filePath))
	stat, err := os.Stat(fullPath)
	if err != nil || stat.IsDir() || stat.Size() <= 0 || stat.Size() > appDevMaxPrototypeImageBytes {
		return schema.ChatMessagePart{}, false
	}

	payload, err := os.ReadFile(fullPath)
	if err != nil || len(payload) == 0 || len(payload) > appDevMaxPrototypeImageBytes {
		return schema.ChatMessagePart{}, false
	}

	mimeType := http.DetectContentType(payload)
	if !strings.HasPrefix(mimeType, "image/") {
		return schema.ChatMessagePart{}, false
	}

	return schema.ChatMessagePart{
		Type: schema.ChatMessagePartTypeImageURL,
		ImageURL: &schema.ChatMessageImageURL{
			URL:      "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(payload),
			MIMEType: mimeType,
		},
	}, true
}

func buildAppDevUserPrompt(req *appdevapp.ChatManagerRequest) string {
	var builder strings.Builder
	builder.WriteString("用户需求：\n")
	builder.WriteString(strings.TrimSpace(req.Message))

	if existingSource := readExistingAppSource(req.ProjectDir); existingSource != "" {
		builder.WriteString("\n\n当前 src/App.tsx 内容（请在此基础上修改，仍然只输出完整 TSX 文件）：\n")
		builder.WriteString(existingSource)
	}

	if len(req.DataSources) > 0 {
		builder.WriteString("\n\n用户已绑定的数据源名称（仅作为页面文案和结构参考，不代表已经检索数据内容）：\n")
		for _, dataSource := range req.DataSources {
			builder.WriteString("- ")
			builder.WriteString(dataSource.Name)
			if dataSource.Type != "" {
				builder.WriteString("（")
				builder.WriteString(dataSource.Type)
				builder.WriteString("）")
			}
			builder.WriteString("\n")
		}
	}

	if len(req.Attachments) > 0 {
		builder.WriteString("\n\n用户上传的附件已经保存到当前 Vite 项目中，可直接在 src/App.tsx 中引用：\n")
		for _, attachment := range req.Attachments {
			builder.WriteString("- ")
			if attachment.Type == "prototype_image" {
				builder.WriteString("原型图 ")
			}
			builder.WriteString(attachment.Name)
			builder.WriteString("：")
			builder.WriteString(attachment.Path)
			if attachment.MIMEType != "" {
				builder.WriteString("，类型 ")
				builder.WriteString(attachment.MIMEType)
			}
			if attachment.Type == "prototype_image" {
				builder.WriteString("。这是一张原型/截图参考，请优先参考其布局、层级、文案位置和视觉结构；如需在 App.tsx 中展示，可使用 new URL('./assets/uploads/")
				builder.WriteString(filepath.Base(attachment.Path))
				builder.WriteString("', import.meta.url).href")
			} else if attachment.Type == "image" {
				builder.WriteString("。如需在 App.tsx 中展示，可使用 new URL('./assets/uploads/")
				builder.WriteString(filepath.Base(attachment.Path))
				builder.WriteString("', import.meta.url).href")
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func readExistingAppSource(projectDir string) string {
	if strings.TrimSpace(projectDir) == "" {
		return ""
	}

	payload, err := os.ReadFile(filepath.Join(projectDir, "src", "App.tsx"))
	if err != nil {
		return ""
	}

	source := strings.TrimSpace(string(payload))
	runes := []rune(source)
	if len(runes) > 12000 {
		return string(runes[:12000]) + "\n/* ...内容已截断... */"
	}

	return source
}

func appDevCodeSystemPrompt() string {
	return `你是网页应用开发助手。请只输出一个完整的 React TSX 文件内容，不要输出 Markdown、解释或代码围栏。
约束：
- 文件是 src/App.tsx。
- 必须包含 export default function App() 或 export default App。
- 不要读取 cookie、localStorage、sessionStorage、process.env。
- 不要使用 eval、new Function 或外部脚本。
- 页面需要可直接由 Vite + React 渲染。
- 样式可以使用内联 style，避免依赖额外 CSS 文件。`
}

func extractTSXSource(content string) string {
	source := strings.TrimSpace(content)
	if source == "" {
		return ""
	}

	if start := strings.Index(source, "```"); start >= 0 {
		block := source[start+3:]
		if newline := strings.Index(block, "\n"); newline >= 0 {
			block = block[newline+1:]
		}
		if end := strings.Index(block, "```"); end >= 0 {
			source = block[:end]
		}
	}

	return strings.TrimSpace(source)
}

func isSafeAppSource(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	if !strings.Contains(trimmed, "export default function App") && !strings.Contains(trimmed, "export default App") {
		return false
	}

	lower := strings.ToLower(trimmed)
	blocked := []string{
		"document.cookie",
		"localstorage",
		"sessionstorage",
		"process.env",
		"eval(",
		"new function",
		"<script",
		"from './",
		"from \"./",
		"import './",
		"import \"./",
	}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			return false
		}
	}

	return true
}

func appDevWriteSummary(filePath string, generatedByModel bool) string {
	if generatedByModel {
		return "模型已生成并更新 " + filePath
	}

	return "模型生成不可用，已使用安全模板更新 " + filePath
}

func generatedAppSource(message string) string {
	requirement := strings.TrimSpace(message)
	if requirement == "" {
		requirement = "构建一个清晰、可用的网页应用"
	}
	if len([]rune(requirement)) > 280 {
		requirement = string([]rune(requirement)[:280]) + "..."
	}

	return fmt.Sprintf(`const requirement = %s;

const highlights = [
  '清晰的信息层级',
  '可直接预览的响应式页面',
  '适合继续通过 AI 迭代',
];

export default function App() {
  return (
    <main
      style={{
        minHeight: '100vh',
        padding: '64px 24px',
        color: '#172033',
        background:
          'radial-gradient(circle at top left, #dbeafe 0, transparent 32%%), linear-gradient(135deg, #f8fafc 0%%, #eef2ff 100%%)',
        fontFamily: 'Avenir Next, PingFang SC, sans-serif',
      }}
    >
      <section
        style={{
          maxWidth: 960,
          margin: '0 auto',
          padding: 40,
          border: '1px solid rgba(37, 99, 235, 0.14)',
          borderRadius: 28,
          background: 'rgba(255,255,255,0.86)',
          boxShadow: '0 24px 80px rgba(30, 64, 175, 0.14)',
        }}
      >
        <p style={{ margin: 0, color: '#2563eb', fontWeight: 700 }}>
          AI Generated Web App
        </p>
        <h1 style={{ margin: '14px 0', fontSize: 44, lineHeight: 1.12 }}>
          网页应用已生成
        </h1>
        <p style={{ maxWidth: 720, color: '#475569', fontSize: 17, lineHeight: 1.8 }}>
          {requirement}
        </p>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
            gap: 16,
            marginTop: 32,
          }}
        >
          {highlights.map(item => (
            <div
              key={item}
              style={{
                padding: 18,
                borderRadius: 18,
                background: '#f8fafc',
                border: '1px solid #e2e8f0',
                fontWeight: 600,
              }}
            >
              {item}
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
`, strconv.Quote(requirement))
}

func (b *ChatBroker) broadcast(key string, event *appdevapp.ChatEvent) {
	b.mu.Lock()
	subscribers := make([]chan *appdevapp.ChatEvent, 0, len(b.subscribers[key]))
	for ch := range b.subscribers[key] {
		subscribers = append(subscribers, ch)
	}
	b.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *ChatBroker) isRunning(key string, requestID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	status := b.status[key]
	return status != nil && status.Running && status.RequestID == requestID
}

func (b *ChatBroker) clearCancel(key string) {
	b.mu.Lock()
	delete(b.cancels, key)
	b.mu.Unlock()
}

func chatKey(req *appdevapp.ChatManagerRequest) string {
	return req.SpaceID + ":" + req.ProjectID
}
