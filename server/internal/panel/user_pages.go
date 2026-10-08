package panel

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"spsatech/helpdesk/internal/auth"
	"spsatech/helpdesk/internal/effcon"
	"spsatech/helpdesk/internal/store"
)

// Техподдержка заводит и редактирует сотрудников; учётки техподдержки
// и администраторов, а также роли меняет только администратор.
func canManage(actor *store.User, target store.Role) bool {
	return actor.Role == store.RoleAdmin || target == store.RoleEmployee
}

type usersData struct {
	Users      []store.User
	Query      string
	SyncOn     bool
	LastSync   *effcon.LastRun
	SyncReport *effcon.Report // результат только что запущенной синхронизации
}

func (p *Panel) usersPage(w http.ResponseWriter, r *http.Request) {
	p.showUsers(w, r, http.StatusOK, "", nil)
}

func (p *Panel) showUsers(w http.ResponseWriter, r *http.Request, status int, errMsg string, rep *effcon.Report) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	users, err := p.store.ListUsers(r.Context(), q)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	d := usersData{Users: users, Query: q, SyncOn: p.sync != nil, SyncReport: rep}
	if d.SyncOn {
		if d.LastSync, err = effcon.Last(r.Context(), p.pool); err != nil {
			p.serverError(w, r, err)
			return
		}
	}
	pg := page{Title: "Сотрудники", Nav: "users", Error: errMsg, Data: d}
	if rep != nil {
		pg.Flash = flashText["sync"]
	}
	p.render(w, r, "users", status, pg)
}

func (p *Panel) syncNow(w http.ResponseWriter, r *http.Request) {
	if p.sync == nil {
		p.notFound(w, r)
		return
	}
	rep, err := p.sync.RunNow(r.Context())
	if err != nil {
		p.showUsers(w, r, http.StatusOK, "Синхронизация не выполнена: "+err.Error(), nil)
		return
	}
	p.showUsers(w, r, http.StatusOK, "", rep)
}

type userForm struct {
	ID         int64
	Login      string
	FullName   string
	Department string
	Phone      string
	Position   string
	Role       store.Role
	IsActive   bool
	IsNew      bool
	External   bool // данные ведутся в effcon
	CanEdit    bool
	CanRole    bool
	IsSelf     bool
	Roles      []store.Role
}

var allRoles = []store.Role{store.RoleEmployee, store.RoleSupport, store.RoleAdmin}

func (p *Panel) userNewPage(w http.ResponseWriter, r *http.Request) {
	actor := current(r).user
	p.renderUserForm(w, r, http.StatusOK, "", userForm{
		IsNew: true, IsActive: true, Role: store.RoleEmployee,
		CanEdit: true, CanRole: actor.Role == store.RoleAdmin,
	})
}

var loginRe = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

func readUserForm(r *http.Request) userForm {
	return userForm{
		Login:      strings.ToLower(strings.TrimSpace(r.PostFormValue("login"))),
		FullName:   strings.TrimSpace(r.PostFormValue("full_name")),
		Department: strings.TrimSpace(r.PostFormValue("department")),
		Phone:      strings.TrimSpace(r.PostFormValue("phone")),
		Position:   strings.TrimSpace(r.PostFormValue("position")),
		Role:       store.Role(r.PostFormValue("role")),
		IsActive:   r.PostFormValue("is_active") == "on",
	}
}

func validateUser(f userForm) string {
	switch {
	case f.FullName == "":
		return "Укажите ФИО"
	case utf8.RuneCountInString(f.FullName) > 200, utf8.RuneCountInString(f.Department) > 200,
		utf8.RuneCountInString(f.Position) > 200,
		utf8.RuneCountInString(f.Phone) > 50:
		return "Слишком длинное значение в одном из полей"
	case !f.Role.Valid():
		return "Выберите роль"
	}
	return ""
}

func (p *Panel) userCreate(w http.ResponseWriter, r *http.Request) {
	actor := current(r).user
	f := readUserForm(r)
	f.IsNew, f.CanEdit, f.CanRole, f.IsActive = true, true, actor.Role == store.RoleAdmin, true
	if !f.CanRole {
		f.Role = store.RoleEmployee
	}
	msg := validateUser(f)
	if msg == "" && !loginRe.MatchString(f.Login) {
		msg = "Логин: 3–64 символа, латинские буквы, цифры, точка, дефис или подчёркивание"
	}
	if msg != "" {
		p.renderUserForm(w, r, http.StatusBadRequest, msg, f)
		return
	}

	password := tempPassword()
	hash, err := auth.HashPassword(password)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	u, err := p.store.CreateUser(r.Context(), store.NewUser{
		Login: f.Login, PasswordHash: hash, FullName: f.FullName, Department: f.Department,
		Phone: f.Phone, Position: f.Position, Role: f.Role, MustChangePassword: true,
	})
	if errors.Is(err, store.ErrDuplicate) {
		p.renderUserForm(w, r, http.StatusConflict, "Логин «"+f.Login+"» уже занят", f)
		return
	}
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	p.showPassword(w, r, u, password, true)
}

func (p *Panel) userEditPage(w http.ResponseWriter, r *http.Request) {
	u, ok := p.loadUser(w, r)
	if !ok {
		return
	}
	p.renderUserForm(w, r, http.StatusOK, "", p.formFor(current(r).user, u))
}

func (p *Panel) formFor(actor, u *store.User) userForm {
	return userForm{
		ID: u.ID, Login: u.Login, FullName: u.FullName, Department: u.Department, Phone: u.Phone,
		Position: u.Position, Role: u.Role, IsActive: u.IsActive, External: u.IsExternal(),
		CanEdit: canManage(actor, u.Role),
		CanRole: actor.Role == store.RoleAdmin && actor.ID != u.ID,
		IsSelf:  actor.ID == u.ID,
	}
}

func (p *Panel) userUpdate(w http.ResponseWriter, r *http.Request) {
	actor := current(r).user
	u, ok := p.loadUser(w, r)
	if !ok {
		return
	}
	base := p.formFor(actor, u)
	if !base.CanEdit {
		http.Error(w, "Недостаточно прав", http.StatusForbidden)
		return
	}
	f := readUserForm(r)
	f.ID, f.Login, f.CanEdit, f.CanRole, f.IsSelf = u.ID, u.Login, true, base.CanRole, base.IsSelf
	if !f.CanRole {
		f.Role = u.Role
	}
	if f.IsSelf {
		f.IsActive = true // себя заблокировать нельзя
	}
	f.External = u.IsExternal()
	if f.External {
		// ФИО, должность, подразделение и активность приходят из effcon.
		f.FullName, f.Department, f.Position, f.IsActive = u.FullName, u.Department, u.Position, u.IsActive
	}
	if msg := validateUser(f); msg != "" {
		p.renderUserForm(w, r, http.StatusBadRequest, msg, f)
		return
	}
	err := p.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{
		FullName: f.FullName, Department: f.Department, Phone: f.Phone, Position: f.Position,
		Role: f.Role, IsActive: f.IsActive,
	})
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("%s/users/%d?ok=saved", prefix, u.ID), http.StatusSeeOther)
}

func (p *Panel) userResetPassword(w http.ResponseWriter, r *http.Request) {
	actor := current(r).user
	u, ok := p.loadUser(w, r)
	if !ok {
		return
	}
	if !canManage(actor, u.Role) || actor.ID == u.ID || u.IsExternal() {
		http.Error(w, "Недостаточно прав", http.StatusForbidden)
		return
	}
	password := tempPassword()
	hash, err := auth.HashPassword(password)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	if _, err := p.store.SetPassword(r.Context(), u.ID, hash, true); err != nil {
		p.serverError(w, r, err)
		return
	}
	p.showPassword(w, r, u, password, false)
}

func (p *Panel) loadUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, ok := pathID(r)
	if !ok {
		p.notFound(w, r)
		return nil, false
	}
	u, err := p.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		p.notFound(w, r)
		return nil, false
	}
	if err != nil {
		p.serverError(w, r, err)
		return nil, false
	}
	return u, true
}

func (p *Panel) renderUserForm(w http.ResponseWriter, r *http.Request, status int, errMsg string, f userForm) {
	f.Roles = allRoles
	title := "Новый пользователь"
	if !f.IsNew {
		title = f.FullName
	}
	p.render(w, r, "user", status, page{Title: title, Nav: "users", Error: errMsg, Data: f})
}

type passwordData struct {
	User     *store.User
	Password string
	Created  bool
}

// showPassword показывает временный пароль один раз — в базе хранится только хеш.
func (p *Panel) showPassword(w http.ResponseWriter, r *http.Request, u *store.User, password string, created bool) {
	w.Header().Set("Cache-Control", "no-store")
	p.render(w, r, "temp_password", http.StatusOK, page{
		Title: "Временный пароль", Nav: "users", Data: passwordData{User: u, Password: password, Created: created},
	})
}

// tempPassword — 10 символов без похожих друг на друга (0/O, 1/l/I).
func tempPassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 10)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}
