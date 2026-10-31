package main

import (
	"net/http"
	"strings"
)

const (
	defaultSiteTitle    = "KeySprint"
	defaultSiteTagline  = "键跃竞赛管理"
	maxSiteTitleRunes   = 40
	maxSiteTaglineRunes = 60
	maxSiteICPRunes     = 40
)

// siteConfig is the public appearance of the deployment: the brand shown in
// the sidebar and login pages plus the ICP filing number in the footer. It is
// stored in app_meta so the appearance can be changed from the admin console
// without rebuilding or editing code.
type siteConfig struct {
	Title   string `json:"title"`
	Tagline string `json:"tagline"`
	ICP     string `json:"icp"`
}

// loadSiteConfig applies appearance settings saved through the admin console.
// Missing keys keep the built-in defaults; the ICP number defaults to empty so
// a fresh deployment never displays somebody else's filing number.
func (a *app) loadSiteConfig() {
	a.siteTitle = defaultSiteTitle
	a.siteTagline = defaultSiteTagline
	a.icpNumber = ""
	values := map[string]*string{
		"site_title":   &a.siteTitle,
		"site_tagline": &a.siteTagline,
		"icp_number":   &a.icpNumber,
	}
	for key, target := range values {
		var stored string
		if err := a.db.QueryRow("SELECT value FROM app_meta WHERE key=?", key).Scan(&stored); err == nil {
			*target = strings.TrimSpace(stored)
		}
	}
	if a.siteTitle == "" {
		a.siteTitle = defaultSiteTitle
	}
}

func (a *app) currentSite() siteConfig {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	title := a.siteTitle
	if title == "" {
		title = defaultSiteTitle
	}
	return siteConfig{Title: title, Tagline: a.siteTagline, ICP: a.icpNumber}
}

// getSite is public: every page needs the brand and footer before login.
func (a *app) getSite(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.currentSite())
}

func (a *app) updateSite(w http.ResponseWriter, r *http.Request) {
	if !mustAdmin(w, r) {
		return
	}
	var in siteConfig
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Tagline = strings.TrimSpace(in.Tagline)
	in.ICP = strings.TrimSpace(in.ICP)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "网站标题不能为空")
		return
	}
	if len([]rune(in.Title)) > maxSiteTitleRunes {
		writeError(w, http.StatusBadRequest, "网站标题不能超过 40 个字符")
		return
	}
	if len([]rune(in.Tagline)) > maxSiteTaglineRunes {
		writeError(w, http.StatusBadRequest, "网站副标题不能超过 60 个字符")
		return
	}
	if len([]rune(in.ICP)) > maxSiteICPRunes {
		writeError(w, http.StatusBadRequest, "备案号不能超过 40 个字符")
		return
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存系统配置失败")
		return
	}
	for key, value := range map[string]string{
		"site_title":   in.Title,
		"site_tagline": in.Tagline,
		"icp_number":   in.ICP,
	} {
		if _, err = tx.Exec("INSERT INTO app_meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value); err != nil {
			_ = tx.Rollback()
			writeError(w, http.StatusInternalServerError, "保存系统配置失败")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "保存系统配置失败")
		return
	}

	a.configMu.Lock()
	a.siteTitle = in.Title
	a.siteTagline = in.Tagline
	a.icpNumber = in.ICP
	a.configMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "site": in})
}
