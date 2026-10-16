package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/HammerMeetNail/nabu/internal/apikey"
	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/middleware"
)

// APIKeyHandler manages a user's personal API keys from Settings. Keys can
// only be listed, created, or revoked with a browser session, never with
// another key.
type APIKeyHandler struct {
	service *apikey.Service
	logger  audit.Logger
}

func NewAPIKeyHandler(service *apikey.Service, logger audit.Logger) *APIKeyHandler {
	return &APIKeyHandler{service: service, logger: logger}
}

func (h *APIKeyHandler) sessionUser(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if middleware.APIKeyID(r.Context()) != "" {
		writeError(w, http.StatusForbidden, "manage API keys from Settings")
		return false
	}
	if _, ok := middleware.CurrentUser(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (h *APIKeyHandler) Keys(w http.ResponseWriter, r *http.Request) {
	if !h.sessionUser(w, r) {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		keys, err := h.service.List(r.Context(), user)
		if err != nil {
			writeServerError(w, "API key list failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid key request")
			return
		}
		key, token, err := h.service.Create(r.Context(), user, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, apikey.ErrInvalidInput):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, apikey.ErrTooManyKeys):
				writeError(w, http.StatusConflict, err.Error())
			case errors.Is(err, apikey.ErrInvalidKey), errors.Is(err, apikey.ErrNotFound):
				writeError(w, http.StatusUnauthorized, "sign in again to create a key")
			default:
				writeServerError(w, "API key creation failed", err)
			}
			return
		}
		audit.Emit(r.Context(), h.logger, "api_key.created", map[string]string{"key_id": key.ID})
		writeJSON(w, http.StatusCreated, map[string]any{"key": key, "token": token})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if !h.sessionUser(w, r) {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	id := r.PathValue("id")
	if err := h.service.Revoke(r.Context(), user, id); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		writeServerError(w, "API key revocation failed", err)
		return
	}
	audit.Emit(r.Context(), h.logger, "api_key.revoked", map[string]string{"key_id": id})
	w.WriteHeader(http.StatusNoContent)
}
