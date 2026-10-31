package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type aiSettings struct {
	BaseURL         string   `json:"baseUrl"`
	Model           string   `json:"model"`
	ReasoningEffort string   `json:"reasoningEffort"`
	HasKey          bool     `json:"hasKey"`
	KeyHint         string   `json:"keyHint"`
	Models          []string `json:"models"`
	ModelsBase      string   `json:"modelsBase"`
	ModelsUpdatedAt string   `json:"modelsUpdatedAt"`
}

func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func (a *app) getAIConfig(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	cfg := a.aiConfigSnapshot()
	cached := a.aiModelsSnapshot()
	writeJSON(w, http.StatusOK, aiSettings{
		BaseURL:         cfg.BaseURL,
		Model:           cfg.Model,
		ReasoningEffort: cfg.ReasoningEffort,
		HasKey:          cfg.APIKey != "",
		KeyHint:         maskAPIKey(cfg.APIKey),
		Models:          cached.Models,
		ModelsBase:      cached.Base,
		ModelsUpdatedAt: cached.At,
	})
}

func (a *app) updateAIConfig(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in struct {
		BaseURL         string `json:"baseUrl"`
		APIKey          string `json:"apiKey"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoningEffort"`
		ClearKey        bool   `json:"clearKey"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	model := strings.TrimSpace(in.Model)
	effort := strings.ToLower(strings.TrimSpace(in.ReasoningEffort))
	key := strings.TrimSpace(in.APIKey)
	if base != "" {
		parsed, err := url.Parse(base)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			writeError(w, http.StatusBadRequest, "AI 接口地址必须是有效的 http(s) 地址（例如 https://example.com/v1）")
			return
		}
	}
	if model == "" {
		model = defaultAIModel
	}
	if !validReasoningEffort(effort) {
		writeError(w, http.StatusBadRequest, "思考强度只能是 low、medium 或 high")
		return
	}
	old := a.aiConfigSnapshot()
	if key == "" && !in.ClearKey {
		key = old.APIKey // 留空表示不修改已保存的密钥
	}
	if in.ClearKey {
		key = ""
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存 AI 配置失败")
		return
	}
	values := map[string]string{
		"ai_base_url":         base,
		"ai_model":            model,
		"ai_reasoning_effort": effort,
	}
	if key != "" || in.ClearKey {
		values["ai_api_key"] = key
	}
	for name, value := range values {
		if _, err = tx.Exec("INSERT INTO app_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", name, value); err != nil {
			_ = tx.Rollback()
			writeError(w, http.StatusInternalServerError, "保存 AI 配置失败")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "保存 AI 配置失败")
		return
	}
	a.configMu.Lock()
	a.aiBaseURL, a.aiModel, a.aiReasoningEffort = base, model, effort
	if key != "" || in.ClearKey {
		a.aiAPIKey = key
	}
	a.configMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *app) probeAIModelsHandler(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cfg := a.aiConfigSnapshot()
	models, err := a.probeAIModels(ctx, cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	models = sanitizeAIModels(models)
	// A successful probe is the only writer of the cache: it replaces whatever
	// was stored before, so the dropdown always reflects the latest probe.
	if err := a.saveAIModelsCache(ctx, cfg.BaseURL, models); err != nil {
		log.Printf("缓存 AI 模型列表失败: %v", err)
	}
	cached := a.aiModelsSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"models": models, "count": len(models), "base": cached.Base, "updatedAt": cached.At,
	})
}

func (a *app) generateAIArticle(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in struct {
		Topic    string `json:"topic"`
		Style    string `json:"style"`
		Extra    string `json:"extra"`
		MinChars int    `json:"minChars"`
		MaxChars int    `json:"maxChars"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Topic = strings.TrimSpace(in.Topic)
	in.Style = strings.TrimSpace(in.Style)
	in.Extra = strings.TrimSpace(in.Extra)
	if in.Topic == "" {
		writeError(w, http.StatusBadRequest, "请填写文章主题")
		return
	}
	if utf8.RuneCountInString(in.Topic) > aiMaxTopicRunes {
		writeError(w, http.StatusBadRequest, "主题不能超过 200 个字符")
		return
	}
	if in.MinChars == 0 {
		in.MinChars = 300
	}
	if in.MaxChars == 0 {
		in.MaxChars = 600
	}
	if in.MinChars < aiMinBodyChars {
		in.MinChars = aiMinBodyChars
	}
	if in.MaxChars > aiMaxBodyChars {
		in.MaxChars = aiMaxBodyChars
	}
	if in.MinChars > in.MaxChars {
		writeError(w, http.StatusBadRequest, "字数下限不能大于上限")
		return
	}
	if in.MaxChars-in.MinChars > 3000 {
		writeError(w, http.StatusBadRequest, "字数范围跨度过大，请缩小到 3000 字以内")
		return
	}
	cfg := a.aiConfigSnapshot()
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		writeError(w, http.StatusBadRequest, "尚未配置 AI 接口，请先到「系统配置 → AI 文章生成」填写接口地址与密钥")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), aiRequestTimeout)
	defer cancel()
	article, err := a.generateArticle(ctx, cfg, aiGenerateInput{
		Topic: in.Topic, Style: in.Style, Extra: in.Extra, MinChars: in.MinChars, MaxChars: in.MaxChars,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	u := currentUser(r)
	res, err := a.db.Exec("INSERT INTO articles(teacher_id,title,content,source) VALUES(?,?,?, 'ai')", u.ID, article.Title, article.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存生成的文章失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "id": id, "title": article.Title, "chars": article.Chars, "model": cfg.Model,
	})
}
