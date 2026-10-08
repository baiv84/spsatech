// Package push отправляет уведомления через Firebase Cloud Messaging (HTTP v1).
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ErrInvalidToken — токен устройства больше не действует (приложение удалено,
// переустановлено и т. п.); его нужно забыть.
var ErrInvalidToken = errors.New("device token is no longer valid")

type FCM struct {
	client    *http.Client
	projectID string
}

// NewFCM читает ключ сервисного аккаунта Firebase.
func NewFCM(ctx context.Context, credentialsJSON []byte) (*FCM, error) {
	creds, err := google.CredentialsFromJSONWithType(ctx, credentialsJSON, google.ServiceAccount,
		"https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, fmt.Errorf("firebase credentials: %w", err)
	}
	if creds.ProjectID == "" {
		return nil, errors.New("firebase credentials: no project_id")
	}
	client := oauth2.NewClient(ctx, creds.TokenSource)
	client.Timeout = 15 * time.Second
	return &FCM{client: client, projectID: creds.ProjectID}, nil
}

// Send отправляет data-сообщение: уведомление рисует само приложение.
func (f *FCM) Send(ctx context.Context, token string, data map[string]string) error {
	payload, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": token,
			"data":  data,
			"android": map[string]any{
				"priority": "HIGH",
				"ttl":      "86400s",
			},
		},
	})
	if err != nil {
		return err
	}
	url := "https://fcm.googleapis.com/v1/projects/" + f.projectID + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var e struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	json.Unmarshal(body, &e)
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return ErrInvalidToken
		}
	}
	if resp.StatusCode == http.StatusNotFound ||
		(resp.StatusCode == http.StatusBadRequest && strings.Contains(e.Error.Message, "registration token")) {
		return ErrInvalidToken
	}
	return fmt.Errorf("fcm %d %s: %s", resp.StatusCode, e.Error.Status, e.Error.Message)
}
