package panel

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"spsatech/helpdesk/internal/store"
)

type period struct {
	From, To time.Time // To — исключительно (начало следующего дня)
	Preset   string
}

func (p period) FromStr() string { return p.From.Format("2006-01-02") }
func (p period) ToStr() string   { return p.To.AddDate(0, 0, -1).Format("2006-01-02") }
func (p period) Label() string {
	return p.From.Format("02.01.2006") + " — " + p.To.AddDate(0, 0, -1).Format("02.01.2006")
}

type preset struct{ Key, Label string }

var presets = []preset{
	{"month", "Этот месяц"}, {"prev-month", "Прошлый месяц"},
	{"quarter", "Этот квартал"}, {"year", "Этот год"},
}

// readPeriod: ?preset=… или ?from=YYYY-MM-DD&to=YYYY-MM-DD (включительно); по умолчанию — текущий месяц.
func readPeriod(r *http.Request) period {
	now := time.Now()
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local) }
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	q := r.URL.Query()

	if from, err1 := time.ParseInLocation("2006-01-02", q.Get("from"), time.Local); err1 == nil {
		if to, err2 := time.ParseInLocation("2006-01-02", q.Get("to"), time.Local); err2 == nil && !to.Before(from) &&
			to.Sub(from) < 5*366*24*time.Hour {
			return period{From: from, To: to.AddDate(0, 0, 1)}
		}
	}
	switch q.Get("preset") {
	case "prev-month":
		return period{From: monthStart.AddDate(0, -1, 0), To: monthStart, Preset: "prev-month"}
	case "quarter":
		qs := time.Date(now.Year(), time.Month((int(now.Month())-1)/3*3+1), 1, 0, 0, 0, 0, time.Local)
		return period{From: qs, To: day(now).AddDate(0, 0, 1), Preset: "quarter"}
	case "year":
		return period{From: time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local), To: day(now).AddDate(0, 0, 1), Preset: "year"}
	}
	return period{From: monthStart, To: day(now).AddDate(0, 0, 1), Preset: "month"}
}

type reportsData struct {
	Period       period
	Presets      []preset
	Summary      store.ReportSummary
	Departments  []store.ReportRow
	Assignees    []store.ReportRow
	MaxDeptCount int
}

func (p *Panel) reportsPage(w http.ResponseWriter, r *http.Request) {
	per := readPeriod(r)
	d := reportsData{Period: per, Presets: presets}
	var err error
	if d.Summary, err = p.store.ReportSummary(r.Context(), per.From, per.To); err != nil {
		p.serverError(w, r, err)
		return
	}
	if d.Departments, err = p.store.ReportByDepartment(r.Context(), per.From, per.To); err != nil {
		p.serverError(w, r, err)
		return
	}
	if d.Assignees, err = p.store.ReportByAssignee(r.Context(), per.From, per.To); err != nil {
		p.serverError(w, r, err)
		return
	}
	for _, row := range d.Departments {
		d.MaxDeptCount = max(d.MaxDeptCount, row.Created, row.Resolved)
	}
	p.render(w, r, "reports", http.StatusOK, page{Title: "Отчёты", Nav: "reports", Data: d})
}

// reportsCSV выгружает заявки периода. Разделитель «;» и BOM — чтобы русский
// Excel открыл файл двойным щелчком без мастера импорта и «кракозябр».
func (p *Panel) reportsCSV(w http.ResponseWriter, r *http.Request) {
	per := readPeriod(r)
	rows, err := p.store.ExportTickets(r.Context(), per.From, per.To)
	if err != nil {
		p.serverError(w, r, err)
		return
	}
	name := fmt.Sprintf("zayavki_%s_%s.csv", per.FromStr(), per.ToStr())
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.Write([]string{"№", "Создана", "Тема", "Место", "Автор", "Подразделение", "Статус", "Исполнитель",
		"Первый ответ", "Решена", "До первого ответа, ч", "До решения, ч", "Сообщений"})
	ts := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Local().Format("02.01.2006 15:04")
	}
	hours := func(from time.Time, to *time.Time) string {
		if to == nil {
			return ""
		}
		// Запятая — десятичный разделитель для русского Excel.
		return formatHours(to.Sub(from).Hours())
	}
	for _, t := range rows {
		cw.Write([]string{
			strconv.FormatInt(t.ID, 10), ts(&t.CreatedAt), csvSafe(t.Title), csvSafe(t.Location), csvSafe(t.Author),
			csvSafe(t.Department), statusLabel(t.Status), csvSafe(t.Assignee), ts(t.FirstResponseAt), ts(t.ResolvedAt),
			hours(t.CreatedAt, t.FirstResponseAt), hours(t.CreatedAt, t.ResolvedAt), strconv.Itoa(t.Messages),
		})
	}
	cw.Flush()
}

func formatHours(h float64) string {
	s := strconv.FormatFloat(math.Round(h*10)/10, 'f', 1, 64)
	for i := range s {
		if s[i] == '.' {
			return s[:i] + "," + s[i+1:]
		}
	}
	return s
}

// csvSafe не даёт Excel принять текст из заявки за формулу (=, +, -, @ в начале ячейки).
func csvSafe(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@' || s[0] == '\t' || s[0] == '\r') {
		return "'" + s
	}
	return s
}

// humanHours — «35 мин», «4,5 ч», «2,3 дн» для показа медиан.
func humanHours(h *float64) string {
	if h == nil {
		return "—"
	}
	switch v := *h; {
	case v < 1:
		return fmt.Sprintf("%d мин", int(math.Round(v*60)))
	case v < 48:
		return formatHours(v) + " ч"
	default:
		return formatHours(v/24) + " дн"
	}
}
