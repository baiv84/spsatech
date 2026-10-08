package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"spsatech/helpdesk/internal/store"
)

type ctxKey struct{}

func currentUser(r *http.Request) *store.User {
	return r.Context().Value(ctxKey{}).(*store.User)
}

type authOption int

const allowPendingPassword authOption = 1

// authed проверяет токен и загружает пользователя из базы при каждом запросе,
// поэтому блокировка или смена пароля действуют сразу.
func (s *Server) authed(h http.HandlerFunc, opts ...authOption) http.Handler {
	allowPending := len(opts) > 0 && opts[0] == allowPendingPassword
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Требуется вход")
			return
		}
		userID, version, err := s.tokens.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Сессия истекла, войдите снова")
			return
		}
		user, err := s.store.UserByID(r.Context(), userID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && (!user.IsActive || user.TokenVersion != version)) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Сессия истекла, войдите снова")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		if user.MustChangePassword && !allowPending {
			writeError(w, http.StatusForbidden, "password_change_required", "Необходимо сменить пароль")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	})
}
