package panel

import (
	"errors"
	"net/http"
	"strings"

	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/store"
)

type loginData struct {
	Login string
	Next  string
}

func (p *Panel) loginPage(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "login", http.StatusOK, page{Title: "Вход", Data: loginData{Next: r.URL.Query().Get("next")}})
}

func (p *Panel) loginSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	login := strings.ToLower(strings.TrimSpace(r.PostFormValue("login")))
	password := r.PostFormValue("password")
	data := loginData{Login: login, Next: r.PostFormValue("next")}
	fail := func(status int, msg string) {
		p.render(w, r, "login", status, page{Title: "Вход", Error: msg, Data: data})
	}

	// Вход не защищён CSRF-токеном (сессии ещё нет), поэтому проверяем источник запроса.
	if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r) {
		fail(http.StatusForbidden, "Запрос отклонён")
		return
	}
	if login == "" || password == "" {
		fail(http.StatusBadRequest, "Введите логин и пароль")
		return
	}
	if p.limiter.Blocked(login) {
		fail(http.StatusTooManyRequests, "Слишком много неудачных попыток. Попробуйте через 15 минут")
		return
	}
	user, err := p.store.UserByLogin(r.Context(), login)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		p.serverError(w, r, err)
		return
	}
	var hash *string
	if user != nil {
		hash = &user.PasswordHash
	}
	if !auth.CheckPasswordOrDummy(hash, password) {
		p.limiter.Fail(login)
		fail(http.StatusUnauthorized, "Неверный логин или пароль")
		return
	}
	if !user.IsActive {
		fail(http.StatusForbidden, "Учётная запись заблокирована")
		return
	}
	if !user.Role.IsStaff() {
		fail(http.StatusForbidden, "Панель доступна только сотрудникам техподдержки. Заявки подаются через мобильное приложение")
		return
	}
	p.limiter.Reset(login)

	token, err := p.tokens.Issue(user.ID, user.TokenVersion)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	p.setCookie(w, r, token)
	if user.MustChangePassword {
		http.Redirect(w, r, prefix+"/password", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, safeNext(data.Next), http.StatusSeeOther)
}

func sameOrigin(origin string, r *http.Request) bool {
	o := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	return o == r.Host
}

func (p *Panel) logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r)
	http.Redirect(w, r, prefix+"/login", http.StatusSeeOther)
}

func (p *Panel) passwordPage(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "password", http.StatusOK, page{Title: "Смена пароля"})
}

func (p *Panel) passwordSubmit(w http.ResponseWriter, r *http.Request) {
	s := current(r)
	current, newPw, repeat := r.PostFormValue("current"), r.PostFormValue("new"), r.PostFormValue("repeat")
	var msg string
	switch {
	case s.user.IsExternal():
		msg = externalPasswordMsg
	case !auth.CheckPassword(s.user.PasswordHash, current):
		msg = "Текущий пароль указан неверно"
	case len([]rune(newPw)) < auth.MinPasswordLength:
		msg = "Новый пароль должен быть не короче 8 символов"
	case newPw != repeat:
		msg = "Пароли не совпадают"
	case newPw == current:
		msg = "Новый пароль должен отличаться от текущего"
	}
	if msg != "" {
		p.render(w, r, "password", http.StatusBadRequest, page{Title: "Смена пароля", Error: msg})
		return
	}
	hash, err := auth.HashPassword(newPw)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	version, err := p.store.SetPassword(r.Context(), s.user.ID, hash, false)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	token, err := p.tokens.Issue(s.user.ID, version)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	p.setCookie(w, r, token)
	http.Redirect(w, r, prefix+"/tickets?ok=password", http.StatusSeeOther)
}

const externalPasswordMsg = "Ваш пароль совпадает с паролем в системе «Оценка эффективности» — меняйте его там"
