package panel

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"spsatech/helpdesk/internal/store"
	"spsatech/helpdesk/internal/upload"
)

type ticketsData struct {
	Tickets []store.StaffTicketSummary
	Counts  map[store.Status]int
	Open    int
	Filter  string // open, all или статус
	Mine    bool
	Query   string
}

func (p *Panel) ticketsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := ticketsData{Filter: q.Get("status"), Mine: q.Get("mine") == "1", Query: strings.TrimSpace(q.Get("q"))}
	if d.Filter == "" {
		d.Filter = "open"
	}

	f := store.TicketFilter{Query: d.Query}
	switch {
	case d.Filter == "open":
		f.Statuses = []store.Status{store.StatusNew, store.StatusInProgress, store.StatusWaiting}
	case d.Filter == "all":
	case store.Status(d.Filter).Valid():
		f.Statuses = []store.Status{store.Status(d.Filter)}
	default:
		d.Filter = "open"
		f.Statuses = []store.Status{store.StatusNew, store.StatusInProgress, store.StatusWaiting}
	}
	if d.Mine {
		f.AssigneeID = current(r).user.ID
	}

	var err error
	if d.Tickets, err = p.store.ListTicketsFiltered(r.Context(), f); err != nil {
		p.serverError(w, r, err)
		return
	}
	if d.Counts, err = p.store.StatusCounts(r.Context()); err != nil {
		p.serverError(w, r, err)
		return
	}
	d.Open = d.Counts[store.StatusNew] + d.Counts[store.StatusInProgress] + d.Counts[store.StatusWaiting]
	p.render(w, r, "tickets", http.StatusOK, page{Title: "Заявки", Nav: "tickets", Data: d})
}

type ticketData struct {
	Ticket *store.Ticket
	Author *store.User
	Staff  []store.User
	Events []store.Event
	Reply  string
}

func (p *Panel) ticketPage(w http.ResponseWriter, r *http.Request) {
	p.showTicket(w, r, http.StatusOK, "", "")
}

func (p *Panel) showTicket(w http.ResponseWriter, r *http.Request, status int, errMsg, reply string) {
	id, ok := pathID(r)
	if !ok {
		p.notFound(w, r)
		return
	}
	t, err := p.store.TicketByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		p.notFound(w, r)
		return
	}
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	author, err := p.store.UserByID(r.Context(), t.Author.ID)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	staff, err := p.store.StaffUsers(r.Context())
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	events, err := p.store.TicketEvents(r.Context(), t.ID)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	p.render(w, r, "ticket", status, page{
		Title: fmt.Sprintf("Заявка №%d", t.ID), Nav: "tickets", Error: errMsg,
		Data: ticketData{Ticket: t, Author: author, Staff: staff, Events: events, Reply: reply},
	})
}

func (p *Panel) ticketReply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		p.notFound(w, r)
		return
	}
	if _, err := p.store.TicketAuthor(r.Context(), id); err != nil {
		p.notFound(w, r)
		return
	}
	form, err := upload.Parse(w, r, p.files)
	var uerr *upload.Error
	if errors.As(err, &uerr) {
		p.showTicket(w, r, uerr.Status, uerr.Message, "")
		return
	}
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	body := strings.TrimSpace(form.Fields["body"])
	newStatus := store.Status(form.Fields["status"])
	if body == "" && len(form.Photos) == 0 {
		p.showTicket(w, r, http.StatusBadRequest, "Напишите ответ или приложите фото", "")
		return
	}
	if utf8.RuneCountInString(body) > 5000 {
		form.Discard(p.files)
		p.showTicket(w, r, http.StatusBadRequest, "Ответ не должен быть длиннее 5000 символов", body)
		return
	}

	user := current(r).user
	if _, err := p.store.AddMessage(r.Context(), id, user, body, form.Photos); err != nil {
		form.Discard(p.files)
		p.serverError(w, r, err)
		return
	}
	// Статус, выбранный вместе с ответом, важнее автоматического.
	if newStatus.Valid() {
		// Сотрудник уже получил уведомление об ответе — второе, о статусе, не шлём.
		if err := p.store.SetStatus(r.Context(), id, user.ID, newStatus, false); err != nil {
			p.serverError(w, r, err)
			return
		}
	}
	redirectTicket(w, r, id, "reply")
}

func (p *Panel) ticketStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	status := store.Status(r.PostFormValue("status"))
	if !ok || !status.Valid() {
		p.notFound(w, r)
		return
	}
	if err := p.store.SetStatus(r.Context(), id, current(r).user.ID, status, true); err != nil {
		p.serverError(w, r, err)
		return
	}
	redirectTicket(w, r, id, "status")
}

func (p *Panel) ticketAssign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		p.notFound(w, r)
		return
	}
	var assignee *int64
	if v := r.PostFormValue("assignee"); v != "" {
		uid, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			p.notFound(w, r)
			return
		}
		u, err := p.store.UserByID(r.Context(), uid)
		if err != nil || !u.Role.IsStaff() || !u.IsActive {
			p.showTicket(w, r, http.StatusBadRequest, "Исполнителем можно назначить только активного сотрудника техподдержки", "")
			return
		}
		assignee = &uid
	}
	if err := p.store.SetAssignee(r.Context(), id, current(r).user.ID, assignee); err != nil {
		p.serverError(w, r, err)
		return
	}
	redirectTicket(w, r, id, "assign")
}

func (p *Panel) attachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		p.notFound(w, r)
		return
	}
	a, err := p.store.AttachmentByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	f, err := p.files.Open(a.StorageKey)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(a.FileName))
	http.ServeContent(w, r, "", a.CreatedAt, f)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func redirectTicket(w http.ResponseWriter, r *http.Request, id int64, ok string) {
	http.Redirect(w, r, fmt.Sprintf("%s/tickets/%d?ok=%s", prefix, id, ok), http.StatusSeeOther)
}

// eventText — строка истории заявки.
func eventText(e store.Event) string {
	switch e.Type {
	case store.EventCreated:
		return "Заявка создана"
	case store.EventStatus:
		return "Статус: " + statusLabel(store.Status(e.From)) + " → " + statusLabel(store.Status(e.To))
	case store.EventAssignee:
		switch {
		case e.To == "":
			return "Снят исполнитель " + e.From
		case e.From == "":
			return "Назначен исполнитель " + e.To
		default:
			return "Исполнитель: " + e.From + " → " + e.To
		}
	}
	return e.Type
}
