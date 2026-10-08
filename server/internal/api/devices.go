package api

import (
	"net/http"
	"strings"
)

type deviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

func readDevice(w http.ResponseWriter, r *http.Request) (deviceRequest, bool) {
	var req deviceRequest
	if !readJSON(w, r, &req) {
		return req, false
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > 4096 {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректный токен устройства")
		return req, false
	}
	if req.Platform != "android" {
		req.Platform = "android"
	}
	return req, true
}

// handleRegisterDevice привязывает телефон к пользователю для push-уведомлений.
func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	req, ok := readDevice(w, r)
	if !ok {
		return
	}
	if err := s.store.RegisterDevice(r.Context(), currentUser(r).ID, req.Token, req.Platform); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUnregisterDevice вызывается при выходе из аккаунта.
func (s *Server) handleUnregisterDevice(w http.ResponseWriter, r *http.Request) {
	req, ok := readDevice(w, r)
	if !ok {
		return
	}
	if err := s.store.UnregisterDevice(r.Context(), currentUser(r).ID, req.Token); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
