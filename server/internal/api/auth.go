package api

import (
	"errors"
	"net/http"
	"strings"

	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/store"
)

type userJSON struct {
	ID                 int64      `json:"id"`
	Login              string     `json:"login"`
	FullName           string     `json:"fullName"`
	Department         string     `json:"department"`
	Departments        []string   `json:"departments"`
	Phone              string     `json:"phone"`
	Position           string     `json:"position"`
	Role               store.Role `json:"role"`
	MustChangePassword bool       `json:"mustChangePassword"`
	// PasswordManagedExternally — пароль меняется в «Оценке эффективности», а не у нас.
	PasswordManagedExternally bool `json:"passwordManagedExternally"`
}

func toUserJSON(u *store.User) userJSON {
	return userJSON{
		ID: u.ID, Login: u.Login, FullName: u.FullName, Department: u.Department, Departments: u.Departments,
		Phone: u.Phone, Position: u.Position, Role: u.Role, MustChangePassword: u.MustChangePassword,
		PasswordManagedExternally: u.IsExternal(),
	}
}

type loginResponse struct {
	Token string   `json:"token"`
	User  userJSON `json:"user"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	login := strings.ToLower(strings.TrimSpace(req.Login))
	if login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "Введите логин и пароль")
		return
	}
	if s.limiter.Blocked(login) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts",
			"Слишком много неудачных попыток. Попробуйте через 15 минут")
		return
	}

	user, err := s.store.UserByLogin(r.Context(), login)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, r, err)
		return
	}
	var hash *string
	if user != nil {
		hash = &user.PasswordHash
	}
	if !auth.CheckPasswordOrDummy(hash, req.Password) {
		s.limiter.Fail(login)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Неверный логин или пароль")
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusForbidden, "user_blocked", "Учётная запись заблокирована. Обратитесь в техподдержку")
		return
	}
	s.limiter.Reset(login)

	token, err := s.tokens.Issue(user.ID, user.TokenVersion)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: toUserJSON(user)})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserJSON(currentUser(r)))
}

// handleChangePassword меняет пароль и возвращает новый токен:
// все остальные сессии пользователя при этом завершаются.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if user.IsExternal() {
		writeError(w, http.StatusBadRequest, "password_managed_externally",
			"Ваш пароль совпадает с паролем в системе «Оценка эффективности» — меняйте его там")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "wrong_password", "Текущий пароль указан неверно")
		return
	}
	if len([]rune(req.NewPassword)) < auth.MinPasswordLength {
		writeError(w, http.StatusBadRequest, "weak_password", "Пароль должен быть не короче 8 символов")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeError(w, http.StatusBadRequest, "weak_password", "Новый пароль должен отличаться от текущего")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		internalError(w, r, err)
		return
	}
	version, err := s.store.SetPassword(r.Context(), user.ID, hash, false)
	if err != nil {
		internalError(w, r, err)
		return
	}
	token, err := s.tokens.Issue(user.ID, version)
	if err != nil {
		internalError(w, r, err)
		return
	}
	user.MustChangePassword = false
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: toUserJSON(user)})
}
