package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"spsatech/helpdesk/internal/store"
	"spsatech/helpdesk/internal/upload"
)

const (
	maxPhotos       = 5
	maxPhotoBytes   = 10 << 20
	maxRequestBytes = maxPhotos*maxPhotoBytes + 1<<20
	maxTitleLen     = 200
	maxDescLen      = 5000
	maxLocationLen  = 200
	maxFieldBytes   = 32 << 10
	photoFieldName  = "photos"
)

func (s *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	authorID := user.ID
	if user.Role.IsStaff() {
		authorID = 0
	}
	tickets, err := s.store.ListTickets(r.Context(), authorID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if tickets == nil {
		tickets = []store.TicketSummary{}
	}
	writeJSON(w, http.StatusOK, tickets)
}

// handleCreateTicket принимает multipart/form-data:
// title, description, location и до 5 файлов в поле photos.
func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	form, ok := s.parseUpload(w, r)
	if !ok {
		return
	}

	title := strings.TrimSpace(form.Fields["title"])
	desc := strings.TrimSpace(form.Fields["description"])
	location := strings.TrimSpace(form.Fields["location"])
	department, deptOK := pickDepartment(user, strings.TrimSpace(form.Fields["department"]))
	var problem string
	switch {
	case !deptOK:
		problem = "Выберите подразделение"
	case title == "":
		problem = "Укажите тему заявки"
	case desc == "":
		problem = "Опишите проблему"
	case utf8.RuneCountInString(title) > maxTitleLen:
		problem = fmt.Sprintf("Тема не должна быть длиннее %d символов", maxTitleLen)
	case utf8.RuneCountInString(desc) > maxDescLen:
		problem = fmt.Sprintf("Описание не должно быть длиннее %d символов", maxDescLen)
	case utf8.RuneCountInString(location) > maxLocationLen:
		problem = fmt.Sprintf("Место не должно быть длиннее %d символов", maxLocationLen)
	}
	if problem != "" {
		form.Discard(s.files)
		writeError(w, http.StatusBadRequest, "validation", problem)
		return
	}

	id, err := s.store.CreateTicket(r.Context(), store.NewTicket{
		AuthorID: user.ID, Title: title, Description: desc, Location: location, Department: department,
	}, form.Photos)
	if err != nil {
		form.Discard(s.files)
		internalError(w, r, err)
		return
	}
	s.writeTicket(w, r, id, http.StatusCreated)
}

func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := s.ticketAccess(w, r)
	if !ok {
		return
	}
	s.writeTicket(w, r, id, http.StatusOK)
}

// handleAddMessage принимает multipart/form-data: body и до 5 фото в поле photos.
func (s *Server) handleAddMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.ticketAccess(w, r)
	if !ok {
		return
	}
	form, ok := s.parseUpload(w, r)
	if !ok {
		return
	}
	body := strings.TrimSpace(form.Fields["body"])
	if body == "" && len(form.Photos) == 0 {
		writeError(w, http.StatusBadRequest, "validation", "Сообщение пустое")
		return
	}
	if utf8.RuneCountInString(body) > maxDescLen {
		form.Discard(s.files)
		writeError(w, http.StatusBadRequest, "validation",
			fmt.Sprintf("Сообщение не должно быть длиннее %d символов", maxDescLen))
		return
	}
	if _, err := s.store.AddMessage(r.Context(), id, currentUser(r), body, form.Photos); err != nil {
		form.Discard(s.files)
		internalError(w, r, err)
		return
	}
	s.writeTicket(w, r, id, http.StatusCreated)
}

func (s *Server) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Файл не найден")
		return
	}
	a, err := s.store.AttachmentByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Файл не найден")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !s.canAccessTicket(w, r, a.TicketID) {
		return
	}
	f, err := s.files.Open(a.StorageKey)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", a.CreatedAt, f)
}

func (s *Server) writeTicket(w http.ResponseWriter, r *http.Request, id int64, status int) {
	t, err := s.store.TicketByID(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, status, t)
}

// ticketAccess разбирает id заявки из пути и проверяет право доступа к ней.
func (s *Server) ticketAccess(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
		return 0, false
	}
	return id, s.canAccessTicket(w, r, id)
}

// canAccessTicket: сотрудник видит только свои заявки, техподдержка — все.
// Чужую заявку отдаём как несуществующую, чтобы не раскрывать её наличие.
func (s *Server) canAccessTicket(w http.ResponseWriter, r *http.Request, ticketID int64) bool {
	authorID, err := s.store.TicketAuthor(r.Context(), ticketID)
	user := currentUser(r)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !user.Role.IsStaff() && authorID != user.ID) {
		writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
		return false
	}
	if err != nil {
		internalError(w, r, err)
		return false
	}
	return true
}

func (s *Server) parseUpload(w http.ResponseWriter, r *http.Request) (*upload.Form, bool) {
	form, err := upload.Parse(w, r, s.files)
	var uerr *upload.Error
	if errors.As(err, &uerr) {
		writeError(w, uerr.Status, uerr.Code, uerr.Message)
		return nil, false
	}
	if err != nil {
		internalError(w, r, err)
		return nil, false
	}
	return form, true
}

// pickDepartment определяет подразделение заявки: из одного — оно само,
// из нескольких — то, что выбрал сотрудник (обязательно из его списка).
// Старые версии приложения поле не присылают — тогда берём первое.
func pickDepartment(u *store.User, chosen string) (string, bool) {
	switch {
	case len(u.Departments) == 0:
		return u.Department, true
	case chosen == "":
		return u.Departments[0], true
	}
	for _, d := range u.Departments {
		if d == chosen {
			return d, true
		}
	}
	return "", false
}
