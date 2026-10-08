package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError отдаёт машинный код ошибки и текст для показа пользователю.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный запрос")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный запрос")
		return false
	}
	return true
}
