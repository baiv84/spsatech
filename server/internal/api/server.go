// Package api реализует HTTP API для мобильного приложения.
package api

import (
	"log/slog"
	"net/http"

	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/files"
	"spsatech/helpdesk/internal/store"
)

type Server struct {
	store   *store.Store
	tokens  *auth.Tokens
	files   *files.Storage
	limiter *auth.LoginLimiter
}

func New(st *store.Store, tokens *auth.Tokens, fs *files.Storage, limiter *auth.LoginLimiter) *Server {
	return &Server{store: st, tokens: tokens, files: fs, limiter: limiter}
}

// Routes собирает обработчики API; extra добавляет другие разделы (веб-панель).
func (s *Server) Routes(extra ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	for _, register := range extra {
		register(mux)
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)

	// Доступны и тем, кому нужно сменить выданный пароль.
	mux.Handle("GET /api/me", s.authed(s.handleMe, allowPendingPassword))
	mux.Handle("POST /api/me/password", s.authed(s.handleChangePassword, allowPendingPassword))

	mux.Handle("GET /api/tickets", s.authed(s.handleListTickets))
	mux.Handle("POST /api/tickets", s.authed(s.handleCreateTicket))
	mux.Handle("GET /api/tickets/{id}", s.authed(s.handleGetTicket))
	mux.Handle("POST /api/tickets/{id}/messages", s.authed(s.handleAddMessage))
	mux.Handle("GET /api/attachments/{id}", s.authed(s.handleGetAttachment))
	mux.Handle("POST /api/devices", s.authed(s.handleRegisterDevice, allowPendingPassword))
	mux.Handle("DELETE /api/devices", s.authed(s.handleUnregisterDevice, allowPendingPassword))

	return recoverer(logRequests(mux))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "err", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
