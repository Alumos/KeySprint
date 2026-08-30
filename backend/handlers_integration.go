package main

import (
	"net/http"
	"strings"
	"time"
)

type integrationConfig struct {
	URL     string `json:"url"`
	Path    string `json:"path"`
	Token   string `json:"token"`
	Timeout string `json:"timeout"`
}

func (a *app) loadIntegrationConfig() error {
	values := map[string]*string{
		"class_system_url":   &a.classSystemURL,
		"class_system_path":  &a.classSystemPath,
		"class_system_token": &a.classSystemToken,
	}
	for key, target := range values {
		var value string
		if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key=?", key).Scan(&value); err == nil {
			*target = value
		}
	}
	var timeout string
	if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key='class_system_timeout'").Scan(&timeout); err == nil {
		if parsed, parseErr := time.ParseDuration(timeout); parseErr == nil && parsed > 0 {
			a.classSystemTimeout = parsed
		}
	}
	a.classSystemURL = strings.TrimRight(a.classSystemURL, "/")
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
		URL: a.classSystemURL, Path: a.classSystemPath, Token: a.classSystemToken, Timeout: a.classSystemTimeout.String(),
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
	in.Path = strings.TrimSpace(in.Path)
	in.Token = strings.TrimSpace(in.Token)
	in.Timeout = strings.TrimSpace(in.Timeout)
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
		"class_system_url": in.URL, "class_system_path": in.Path, "class_system_token": in.Token, "class_system_timeout": in.Timeout,
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
	a.classSystemURL, a.classSystemPath, a.classSystemToken, a.classSystemTimeout = in.URL, in.Path, in.Token, timeout
	a.configMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
