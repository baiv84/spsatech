// Package download — публичная страница скачивания Android-приложения.
//
// APK и его описание (meta.json) лежат в отдельной папке и публикуются
// скриптом deploy/publish-apk.sh без пересборки сервера.
package download

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"rsc.io/qr"
)

const apkName = "helpdesk.apk"

type Meta struct {
	VersionName string `json:"versionName"`
	VersionCode int    `json:"versionCode"`
	// MinVersionCode — версии старее этой обязаны обновиться (0 — обновление необязательное).
	MinVersionCode int       `json:"minVersionCode"`
	SizeBytes      int64     `json:"sizeBytes"`
	SHA256         string    `json:"sha256"`
	PublishedAt    time.Time `json:"publishedAt"`
}

type Handler struct {
	dir string
}

func New(dir string) *Handler { return &Handler{dir: dir} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app", h.page)
	mux.HandleFunc("GET /app/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /app/"+apkName, h.apk)
	mux.HandleFunc("GET /app/qr.png", h.qr)
	mux.HandleFunc("GET /app/version.json", h.version)
}

func (h *Handler) meta() (*Meta, error) {
	b, err := os.ReadFile(filepath.Join(h.dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var m Meta
	return &m, json.Unmarshal(b, &m)
}

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"mb":   func(n int64) string { return fmt.Sprintf("%.1f МБ", float64(n)/(1<<20)) },
	"date": func(t time.Time) string { return t.Local().Format("02.01.2006") },
}).Parse(pageHTML))

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	m, err := h.meta()
	if err != nil && !os.IsNotExist(err) {
		slog.Error("apk meta", "err", err)
	}
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, struct {
		Meta *Meta
		URL  string
	}{m, pageURL(r)}); err != nil {
		slog.Error("download page", "err", err)
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'")
	w.Write(buf.Bytes())
}

func (h *Handler) apk(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(filepath.Join(h.dir, apkName))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	name := "helpdesk.apk"
	if m, err := h.meta(); err == nil && m.VersionName != "" {
		name = "helpdesk-" + m.VersionName + ".apk"
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// version — для проверки обновлений из приложения.
func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	m, err := h.meta()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(struct {
		*Meta
		URL string `json:"url"`
	}{m, "/app/" + apkName})
}

// qr — QR-код со ссылкой на эту страницу: удобно показать с экрана или распечатать.
func (h *Handler) qr(w http.ResponseWriter, r *http.Request) {
	code, err := qr.Encode(pageURL(r), qr.M)
	if err != nil {
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	code.Scale = 8
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(code.PNG())
}

func pageURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/app"
}
