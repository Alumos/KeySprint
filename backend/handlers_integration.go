package main

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

type integrationConfig struct {
	Source  string `json:"source"`
	URL     string `json:"url"`
	Path    string `json:"path"`
	Token   string `json:"token"`
	Timeout string `json:"timeout"`
}

func (a *app) loadIntegrationConfig() error {
	values := map[string]*string{
		"class_source":       &a.classSource,
		"class_system_url":   &a.classSystemURL,
		"class_system_path":  &a.classSystemPath,
		"class_system_token": &a.classSystemToken,
	}
	sourceStored, urlStored := false, false
	for key, target := range values {
		var value string
		if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key=?", key).Scan(&value); err == nil {
			*target = value
			sourceStored = sourceStored || key == "class_source"
			urlStored = urlStored || key == "class_system_url"
		}
	}
	var timeout string
	if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key='class_system_timeout'").Scan(&timeout); err == nil {
		if parsed, parseErr := time.ParseDuration(timeout); parseErr == nil && parsed > 0 {
			a.classSystemTimeout = parsed
		}
	}
	a.classSystemURL = strings.TrimRight(a.classSystemURL, "/")
	// v1.0 stored the endpoint but had no explicit source selector. Preserve
	// its working ClassOrbit behavior on upgrade without overriding fresh env defaults.
	if !sourceStored && urlStored && a.classSystemURL != "" {
		a.classSource = "classorbit"
	}
	a.classSource = normalizeClassSource(a.classSource)
	if a.classSystemPath == "" {
		a.classSystemPath = "/api/integration/classes"
	}
	return nil
}

func (a *app) getIntegrationConfig(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	writeJSON(w, http.StatusOK, integrationConfig{
		Source: a.classSource, URL: a.classSystemURL, Path: a.classSystemPath, Token: a.classSystemToken, Timeout: a.classSystemTimeout.String(),
	})
}

func (a *app) updateIntegrationConfig(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in integrationConfig
	if !decodeJSON(w, r, &in) {
		return
	}
	in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	in.Source = strings.ToLower(strings.TrimSpace(in.Source))
	if in.Source == "" {
		in.Source = "manual"
	}
	if in.Source != "manual" && in.Source != "classorbit" {
		writeError(w, http.StatusBadRequest, "班级名单来源必须是 manual 或 classorbit")
		return
	}
	in.Path = strings.TrimSpace(in.Path)
	in.Token = strings.TrimSpace(in.Token)
	in.Timeout = strings.TrimSpace(in.Timeout)
	if in.Source == "classorbit" {
		if in.URL == "" {
			writeError(w, http.StatusBadRequest, "请填写 ClassOrbit 服务地址")
			return
		}
		parsedURL, err := url.Parse(in.URL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			writeError(w, http.StatusBadRequest, "ClassOrbit 服务地址必须是有效的 http(s) 地址")
			return
		}
		if in.Token == "" {
			writeError(w, http.StatusBadRequest, "请填写与 ClassOrbit 一致的共享密钥")
			return
		}
	}
	if in.Path == "" {
		in.Path = "/api/integration/classes"
	}
	if in.Timeout == "" {
		in.Timeout = "8s"
	}
	timeout, err := time.ParseDuration(in.Timeout)
	if err != nil || timeout <= 0 || timeout > 60*time.Second {
		writeError(w, http.StatusBadRequest, "超时时间格式无效（例如 8s）")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存配置失败")
		return
	}
	for key, value := range map[string]string{
		"class_source": in.Source, "class_system_url": in.URL, "class_system_path": in.Path, "class_system_token": in.Token, "class_system_timeout": in.Timeout,
	} {
		if _, err = tx.Exec("INSERT INTO app_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "保存配置失败")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "保存配置失败")
		return
	}
	a.configMu.Lock()
	a.classSource, a.classSystemURL, a.classSystemPath, a.classSystemToken, a.classSystemTimeout = in.Source, in.URL, in.Path, in.Token, timeout
	a.classSystemETags = map[string]string{}
	a.configMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func normalizeClassSource(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "classorbit") {
		return "classorbit"
	}
	return "manual"
}
