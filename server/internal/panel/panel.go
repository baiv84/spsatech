// Package panel — веб-панель техподдержки (HTML, рендерится на сервере).
package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/effcon"
	"spsatech/helpdesk/internal/files"
	"spsatech/helpdesk/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const (
	cookieName = "hd_panel"
	prefix     = "/panel"
)

type Panel struct {
	store   *store.Store
	tokens  *auth.Tokens
	files   *files.Storage
	limiter *auth.LoginLimiter
	secret  []byte
	ttl     time.Duration
	pages   map[string]*template.Template
	pool    *pgxpool.Pool
	sync    *effcon.Scheduler // nil, если синхронизация не настроена
}

func New(st *store.Store, tokens *auth.Tokens, fs *files.Storage, limiter *auth.LoginLimiter,
	secret []byte, ttl time.Duration, pool *pgxpool.Pool, sync *effcon.Scheduler) (*Panel, error) {
	p := &Panel{store: st, tokens: tokens, files: fs, limiter: limiter, secret: secret, ttl: ttl,
		pool: pool, sync: sync}
	return p, p.parseTemplates()
}

// Register добавляет маршруты панели в mux.
func (p *Panel) Register(mux *http.ServeMux) {
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /panel/static/", http.StripPrefix("/panel/static/", cacheStatic(http.FileServerFS(static))))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix+"/tickets", http.StatusFound)
	})
	mux.HandleFunc("GET /panel/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix+"/tickets", http.StatusFound)
	})
	mux.HandleFunc("GET /panel/login", p.loginPage)
	mux.HandleFunc("POST /panel/login", p.loginSubmit)
	mux.Handle("POST /panel/logout", p.authed(p.logout, allowPending))
	mux.Handle("GET /panel/password", p.authed(p.passwordPage, allowPending))
	mux.Handle("POST /panel/password", p.authed(p.passwordSubmit, allowPending))

	mux.Handle("GET /panel/tickets", p.authed(p.ticketsPage))
	mux.Handle("GET /panel/tickets/{id}", p.authed(p.ticketPage))
	mux.Handle("POST /panel/tickets/{id}/reply", p.authed(p.ticketReply))
	mux.Handle("POST /panel/tickets/{id}/status", p.authed(p.ticketStatus))
	mux.Handle("POST /panel/tickets/{id}/assign", p.authed(p.ticketAssign))
	mux.Handle("GET /panel/attachments/{id}", p.authed(p.attachment))

	mux.Handle("GET /panel/users", p.authed(p.usersPage))
	mux.Handle("GET /panel/users/new", p.authed(p.userNewPage))
	mux.Handle("POST /panel/users/new", p.authed(p.userCreate))
	mux.Handle("GET /panel/users/{id}", p.authed(p.userEditPage))
	mux.Handle("POST /panel/users/{id}", p.authed(p.userUpdate))
	mux.Handle("POST /panel/users/{id}/reset-password", p.authed(p.userResetPassword))
	mux.Handle("POST /panel/sync", p.authed(p.syncNow))
	mux.Handle("GET /panel/reports", p.authed(p.reportsPage))
	mux.Handle("GET /panel/reports.csv", p.authed(p.reportsCSV))
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

// ---- сессия ----

type ctxKey struct{}

type session struct {
	user  *store.User
	token string
}

func current(r *http.Request) *session {
	return r.Context().Value(ctxKey{}).(*session)
}

type option int

const allowPending option = 1

// authed пускает только активных сотрудников техподдержки и администраторов,
// а для POST-запросов дополнительно проверяет CSRF-токен.
func (p *Panel) authed(h http.HandlerFunc, opts ...option) http.Handler {
	pendingOK := len(opts) > 0 && opts[0] == allowPending
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			p.toLogin(w, r)
			return
		}
		userID, version, err := p.tokens.Parse(c.Value)
		if err != nil {
			p.toLogin(w, r)
			return
		}
		user, err := p.store.UserByID(r.Context(), userID)
		if errors.Is(err, store.ErrNotFound) ||
			(err == nil && (!user.IsActive || user.TokenVersion != version || !user.Role.IsStaff())) {
			p.toLogin(w, r)
			return
		}
		if err != nil {
			p.serverError(w, r, err)
			return
		}
		if r.Method == http.MethodPost && !p.validCSRF(r, c.Value) {
			http.Error(w, "Форма устарела. Обновите страницу и повторите.", http.StatusForbidden)
			return
		}
		if user.MustChangePassword && !pendingOK {
			http.Redirect(w, r, prefix+"/password", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, &session{user: user, token: c.Value})
		h(w, r.WithContext(ctx))
	})
}

func (p *Panel) toLogin(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r)
	next := ""
	if r.Method == http.MethodGet {
		next = "?next=" + url.QueryEscape(r.URL.RequestURI())
	}
	http.Redirect(w, r, prefix+"/login"+next, http.StatusSeeOther)
}

func (p *Panel) csrfToken(sessionToken string) string {
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte("csrf:" + sessionToken))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

// validCSRF читает токен из заголовка или поля формы. Для multipart-форм
// поле читается из URL (?_csrf=), чтобы не разбирать тело заранее.
func (p *Panel) validCSRF(r *http.Request, sessionToken string) bool {
	got := r.Header.Get("X-CSRF-Token")
	if got == "" {
		got = r.URL.Query().Get("_csrf")
	}
	if got == "" && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		got = r.PostFormValue("_csrf")
	}
	return hmac.Equal([]byte(got), []byte(p.csrfToken(sessionToken)))
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func (p *Panel) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: prefix,
		MaxAge: int(p.ttl.Seconds()), HttpOnly: true, Secure: secureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: prefix, MaxAge: -1,
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode,
	})
}

// safeNext допускает возврат только на страницы панели.
func safeNext(next string) string {
	if strings.HasPrefix(next, prefix+"/") && !strings.HasPrefix(next, "//") && !strings.Contains(next, "\\") {
		return next
	}
	return prefix + "/tickets"
}

// ---- рендеринг ----

type page struct {
	Title string
	User  *store.User
	CSRF  string
	Nav   string
	Flash string
	Error string
	Data  any
}

func (p *Panel) render(w http.ResponseWriter, r *http.Request, name string, status int, pg page) {
	if s, ok := r.Context().Value(ctxKey{}).(*session); ok {
		pg.User = s.user
		pg.CSRF = p.csrfToken(s.token)
	}
	if pg.Flash == "" {
		pg.Flash = flashText[r.URL.Query().Get("ok")]
	}
	t, ok := p.pages[name]
	if !ok {
		p.serverError(w, r, errors.New("unknown template "+name))
		return
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, "layout", pg); err != nil {
		p.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; img-src 'self' blob:; style-src 'self'; script-src 'self'; form-action 'self'; frame-ancestors 'none'")
	w.WriteHeader(status)
	w.Write([]byte(buf.String()))
}

var flashText = map[string]string{
	"reply":    "Ответ отправлен",
	"status":   "Статус изменён",
	"assign":   "Исполнитель изменён",
	"saved":    "Изменения сохранены",
	"password": "Пароль изменён",
	"sync":     "Синхронизация выполнена",
}

func (p *Panel) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("panel request failed", "path", r.URL.Path, "err", err)
	http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
}

func (p *Panel) notFound(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "notfound", http.StatusNotFound, page{Title: "Не найдено"})
}

func (p *Panel) parseTemplates() error {
	version, err := staticVersion()
	if err != nil {
		return err
	}
	funcs := template.FuncMap{
		// asset добавляет к ссылке хеш содержимого, чтобы после обновления
		// браузеры не держали старые CSS/JS из кэша.
		"asset":       func(name string) string { return "/panel/static/" + name + "?v=" + version },
		"statusLabel": statusLabel,
		"roleLabel":   roleLabel,
		"fmtTime": func(t time.Time) string {
			return t.Local().Format("02.01.2006 15:04")
		},
		"statuses": func() []store.Status { return allStatuses },
		"eqStatus": func(a store.Status, b string) bool { return string(a) == b },
		"hours":    humanHours,
		"event":    eventText,
		"pct": func(n, max int) int {
			if max == 0 {
				return 0
			}
			return n * 100 / max
		},
	}
	layout, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html")
	if err != nil {
		return err
	}
	names, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return err
	}
	p.pages = map[string]*template.Template{}
	for _, n := range names {
		name := strings.TrimSuffix(strings.TrimPrefix(n, "templates/"), ".html")
		if name == "layout" {
			continue
		}
		t, err := template.Must(layout.Clone()).ParseFS(templateFS, n)
		if err != nil {
			return err
		}
		p.pages[name] = t
	}
	return nil
}

func staticVersion() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := staticFS.ReadFile(path)
		h.Write(b)
		return err
	})
	return hex.EncodeToString(h.Sum(nil))[:10], err
}

var allStatuses = []store.Status{
	store.StatusNew, store.StatusInProgress, store.StatusWaiting, store.StatusResolved, store.StatusClosed,
}

func statusLabel(s store.Status) string {
	switch s {
	case store.StatusNew:
		return "Новая"
	case store.StatusInProgress:
		return "В работе"
	case store.StatusWaiting:
		return "Ожидает ответа"
	case store.StatusResolved:
		return "Решена"
	case store.StatusClosed:
		return "Закрыта"
	}
	return string(s)
}

func roleLabel(r store.Role) string {
	switch r {
	case store.RoleEmployee:
		return "Сотрудник"
	case store.RoleSupport:
		return "Техподдержка"
	case store.RoleAdmin:
		return "Администратор"
	}
	return string(r)
}
