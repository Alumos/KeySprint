package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultAIModel     = "gpt-6-luna"
	defaultAIReasoning = "medium"

	// Probed model ids are cached in app_meta so the dropdown survives page
	// reloads and restarts; a successful probe replaces the whole cache.
	aiModelsCacheKey = "ai_models_cache"
	aiModelsCacheMax = 64

	aiMaxResponseBytes = 1 << 20
	aiRequestTimeout   = 120 * time.Second
	aiMinBodyChars     = 100
	aiMaxBodyChars     = 5000
	aiMaxTopicRunes    = 200
)

// aiHTTPClient is reused across requests so upstream TLS connections stay warm;
// generation is the slowest admin action in the app.
var aiHTTPClient = &http.Client{Timeout: aiRequestTimeout}

type aiConfig struct {
	BaseURL         string
	APIKey          string
	Model           string
	ReasoningEffort string
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatRequest struct {
	Model           string      `json:"model"`
	Messages        []aiMessage `json:"messages"`
	Temperature     float64     `json:"temperature,omitempty"`
	MaxTokens       int         `json:"max_tokens,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
}

type generatedArticle struct {
	Title   string
	Content string
	Chars   int
}

type aiGenerateInput struct {
	Topic    string
	Style    string
	Extra    string
	MinChars int
	MaxChars int
}

type aiModelsCache struct {
	Base   string   `json:"base"`
	At     string   `json:"at"`
	Models []string `json:"models"`
}

var aiReasoningLevels = map[string]bool{"low": true, "medium": true, "high": true}

func validReasoningEffort(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || aiReasoningLevels[value]
}

func (a *app) loadAIConfig() {
	// Environment variables seed the defaults; values saved from the console win.
	a.configMu.Lock()
	defer a.configMu.Unlock()
	values := map[string]*string{
		"ai_base_url":         &a.aiBaseURL,
		"ai_api_key":          &a.aiAPIKey,
		"ai_model":            &a.aiModel,
		"ai_reasoning_effort": &a.aiReasoningEffort,
	}
	for key, target := range values {
		var stored string
		if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key=?", key).Scan(&stored); err == nil && strings.TrimSpace(stored) != "" {
			*target = strings.TrimSpace(stored)
		}
	}
	a.aiBaseURL = strings.TrimRight(a.aiBaseURL, "/")
	if a.aiModel == "" {
		a.aiModel = defaultAIModel
	}
	if !validReasoningEffort(a.aiReasoningEffort) {
		a.aiReasoningEffort = defaultAIReasoning
	}
	a.aiModels, a.aiModelsBase, a.aiModelsAt = nil, "", ""
	var cached string
	if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key=?", aiModelsCacheKey).Scan(&cached); err == nil {
		var parsed aiModelsCache
		if json.Unmarshal([]byte(cached), &parsed) == nil {
			a.aiModels = sanitizeAIModels(parsed.Models)
			a.aiModelsBase = parsed.Base
			a.aiModelsAt = parsed.At
		}
	}
}

// aiModelsSnapshot returns the last probed list; it is display-only metadata,
// never used to pick a model automatically.
func (a *app) aiModelsSnapshot() aiModelsCache {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return aiModelsCache{Base: a.aiModelsBase, At: a.aiModelsAt, Models: append([]string(nil), a.aiModels...)}
}

// sanitizeAIModels keeps the cache small and stable: trimmed, unique, bounded
// in both count and length so a hostile gateway cannot bloat the database.
func sanitizeAIModels(models []string) []string {
	seen := make(map[string]bool, len(models))
	out := make([]string, 0, len(models))
	for _, raw := range models {
		name := strings.TrimSpace(raw)
		if name == "" || len([]rune(name)) > 120 || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) >= aiModelsCacheMax {
			break
		}
	}
	return out
}

// saveAIModelsCache replaces the cached list after a successful probe.
func (a *app) saveAIModelsCache(ctx context.Context, base string, models []string) error {
	cache := aiModelsCache{Base: base, At: time.Now().Format("2006-01-02 15:04"), Models: models}
	raw, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if _, err = a.db.ExecContext(ctx, "INSERT INTO app_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", aiModelsCacheKey, string(raw)); err != nil {
		return err
	}
	a.configMu.Lock()
	a.aiModels, a.aiModelsBase, a.aiModelsAt = cache.Models, cache.Base, cache.At
	a.configMu.Unlock()
	return nil
}

func (a *app) aiConfigSnapshot() aiConfig {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return aiConfig{BaseURL: a.aiBaseURL, APIKey: a.aiAPIKey, Model: a.aiModel, ReasoningEffort: a.aiReasoningEffort}
}

func articleChars(text string) int {
	return utf8.RuneCountInString(strings.Join(strings.Fields(text), ""))
}

func upstreamSnippet(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	if utf8.RuneCountInString(text) > limit {
		text = string([]rune(text)[:limit]) + "…"
	}
	return text
}

// probeAIModels lists the model ids advertised by an OpenAI-compatible gateway.
func (a *app) probeAIModels(ctx context.Context, cfg aiConfig) ([]string, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("尚未填写 AI 接口地址")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("尚未填写 API 密钥")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, errors.New("AI 接口地址无效")
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	res, err := aiHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 AI 接口失败：%v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, aiMaxResponseBytes))
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("上游返回 HTTP %d：%s", res.StatusCode, upstreamSnippet(body, 160))
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []string `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("上游返回的模型列表不是有效 JSON")
	}
	ids := make([]string, 0, len(payload.Data)+len(payload.Models))
	for _, item := range payload.Data {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	for _, id := range payload.Models {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil, errors.New("上游没有返回可用模型，请检查接口地址是否正确（需要 OpenAI 兼容的 /v1 地址）")
	}
	return ids, nil
}

type aiUpstreamError struct {
	Status int
	Body   string
}

func (e aiUpstreamError) Error() string {
	return fmt.Sprintf("上游返回 HTTP %d：%s", e.Status, upstreamSnippet([]byte(e.Body), 200))
}

// retryWithoutReasoning reports whether the upstream rejected reasoning_effort
// rather than the request itself. Many OpenAI-compatible gateways do not know
// that field, so callers retry once without it instead of failing the user.
func retryWithoutReasoning(err error) bool {
	var upstream aiUpstreamError
	if !errors.As(err, &upstream) || (upstream.Status != http.StatusBadRequest && upstream.Status != http.StatusUnprocessableEntity) {
		return false
	}
	lower := strings.ToLower(upstream.Body)
	for _, hint := range []string{"reasoning", "unknown parameter", "unsupported", "unrecognized", "extra fields", "extra_forbidden"} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func (a *app) aiChat(ctx context.Context, cfg aiConfig, messages []aiMessage, maxTokens int) (string, error) {
	efforts := []string{}
	if cfg.ReasoningEffort != "" {
		efforts = append(efforts, cfg.ReasoningEffort)
	}
	efforts = append(efforts, "")

	var lastErr error
	for _, effort := range efforts {
		payload := aiChatRequest{
			Model:           cfg.Model,
			Messages:        messages,
			Temperature:     0.8,
			MaxTokens:       maxTokens,
			ReasoningEffort: effort,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return "", errors.New("构造 AI 请求失败")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+"/chat/completions", bytes.NewReader(data))
		if err != nil {
			return "", errors.New("AI 接口地址无效")
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		res, err := aiHTTPClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", errors.New("AI 生成超时，请稍后重试或调低字数")
			}
			return "", fmt.Errorf("连接 AI 接口失败：%v", err)
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, aiMaxResponseBytes))
		res.Body.Close()
		if res.StatusCode/100 != 2 {
			upstreamErr := aiUpstreamError{Status: res.StatusCode, Body: string(body)}
			if effort != "" && retryWithoutReasoning(upstreamErr) {
				lastErr = upstreamErr
				continue
			}
			return "", upstreamErr
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return "", errors.New("上游返回内容不是有效 JSON")
		}
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("上游报错：%s", upstreamSnippet([]byte(parsed.Error.Message), 160))
		}
		if len(parsed.Choices) == 0 {
			return "", errors.New("上游没有返回生成内容")
		}
		content, err := decodeChatContent(parsed.Choices[0].Message.Content)
		if err != nil {
			return "", err
		}
		return content, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("AI 生成失败")
}

func decodeChatContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("上游返回了空内容")
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var builder strings.Builder
		for _, part := range parts {
			builder.WriteString(part.Text)
		}
		if builder.Len() > 0 {
			return builder.String(), nil
		}
	}
	return "", errors.New("无法解析上游返回的生成内容")
}

// buildArticlePrompt asks for a strict JSON payload; parseGeneratedArticle keeps
// working when a model ignores the format and returns plain text instead.
func buildArticlePrompt(in aiGenerateInput) []aiMessage {
	minChars, maxChars := in.MinChars, in.MaxChars
	style := strings.TrimSpace(in.Style)
	if style == "" {
		style = "记叙文"
	}
	extra := strings.TrimSpace(in.Extra)
	if extra != "" {
		extra = "\n- 额外要求：" + extra
	}
	system := "你是中文打字课范文作者，只输出 JSON，不要输出 Markdown、代码块、解释或任何多余文字。"
	user := "请生成一篇适合课堂中文打字练习的范文。\n\n要求：\n" +
		"- 主题：" + in.Topic + "\n" +
		"- 体裁/风格：" + style + "\n" +
		fmt.Sprintf("- 正文长度：%d-%d 个汉字（不含标点更好，允许少量浮动）", minChars, maxChars) + "\n" +
		"- 语言：纯简体中文，不用英文、拼音或列表\n" +
		"- 内容积极健康，语句通顺，适合中小学生阅读" + extra + "\n\n" +
		"只输出如下 JSON：{\"title\":\"不超过 20 字的标题\",\"content\":\"正文\"}"
	return []aiMessage{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

func parseGeneratedArticle(raw string, in aiGenerateInput) (generatedArticle, error) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "\ufeff")
	// Strip one Markdown code fence if the model wrapped its answer.
	if strings.HasPrefix(text, "```") {
		if idx := strings.IndexAny(text, "\n"); idx >= 0 {
			text = text[idx+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
		text = strings.TrimSpace(text)
	}
	var title, content string
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		var payload struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(text[start:end+1]), &payload) == nil {
			title = strings.TrimSpace(payload.Title)
			content = strings.TrimSpace(payload.Content)
		}
	}
	if title == "" || content == "" {
		lines := strings.Split(text, "\n")
		var body []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				if len(body) > 0 {
					body = append(body, "")
				}
				continue
			}
			if title == "" {
				title = strings.TrimSpace(strings.TrimLeft(line, "#*「」“”\"' "))
				title = strings.TrimPrefix(title, "标题：")
				title = strings.TrimPrefix(title, "标题:")
				continue
			}
			body = append(body, line)
		}
		content = strings.TrimSpace(strings.Join(body, "\n"))
	}
	title = strings.TrimSpace(strings.Trim(title, "\"“”' 「」"))
	content = normalizeArticleBody(content)
	if title == "" || content == "" {
		return generatedArticle{}, errors.New("AI 返回内容不完整，请重试")
	}
	if chars := utf8.RuneCountInString(title); chars > 60 {
		title = string([]rune(title)[:60])
	}
	chars := articleChars(content)
	if chars < in.MinChars/2 {
		return generatedArticle{}, fmt.Errorf("生成内容偏短（%d 字），请重试或调低字数下限", chars)
	}
	if chars > in.MaxChars*2 {
		return generatedArticle{}, fmt.Errorf("生成内容过长（%d 字），请重试或调低字数上限", chars)
	}
	return generatedArticle{Title: title, Content: content, Chars: chars}, nil
}

func normalizeArticleBody(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(strings.TrimSpace(line), " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (a *app) generateArticle(ctx context.Context, cfg aiConfig, in aiGenerateInput) (generatedArticle, error) {
	if cfg.Model == "" {
		return generatedArticle{}, errors.New("尚未选择 AI 模型")
	}
	maxTokens := in.MaxChars*2 + 300
	if maxTokens > 4096 {
		maxTokens = 4096
	}
	raw, err := a.aiChat(ctx, cfg, buildArticlePrompt(in), maxTokens)
	if err != nil {
		return generatedArticle{}, err
	}
	return parseGeneratedArticle(raw, in)
}
