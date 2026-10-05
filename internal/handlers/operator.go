package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/operator"
)

type OperatorHandler struct {
	service *operator.Service
	logger  audit.Logger
}

func NewOperatorHandler(service *operator.Service, logger audit.Logger) *OperatorHandler {
	return &OperatorHandler{service: service, logger: logger}
}

func operatorHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization, Cookie")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}

func (h *OperatorHandler) readAccess(w http.ResponseWriter, r *http.Request, scope string) (operator.Access, bool) {
	operatorHeaders(w)
	provided := r.Header.Get("Authorization")
	if provided != "" {
		// A browser cookie must not silently override an invalid bearer token.
		if !strings.HasPrefix(provided, "Bearer ") || len(provided) <= len("Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return operator.Access{}, false
		}
		access, err := h.service.AuthenticateKey(r.Context(), strings.TrimPrefix(provided, "Bearer "), scope)
		if err != nil {
			if !errors.Is(err, operator.ErrInvalidKey) {
				writeServerError(w, "operator key authentication failed", err)
			} else {
				writeError(w, http.StatusUnauthorized, "unauthorized")
			}
			return operator.Access{}, false
		}
		audit.Emit(r.Context(), h.logger, "operator.report_read", map[string]string{"key_id": access.KeyID(), "resource": r.URL.Path})
		return access, true
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return operator.Access{}, false
	}
	if !h.service.IsOwner(user) {
		writeError(w, http.StatusForbidden, "forbidden")
		return operator.Access{}, false
	}
	access, err := h.service.AuthorizeSession(user)
	if err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return operator.Access{}, false
	}
	audit.Emit(r.Context(), h.logger, "operator.report_read", map[string]string{"resource": r.URL.Path})
	return access, true
}

func (h *OperatorHandler) ownerSession(w http.ResponseWriter, r *http.Request) bool {
	operatorHeaders(w)
	if r.Header.Get("Authorization") != "" {
		writeError(w, http.StatusForbidden, "use an operator session")
		return false
	}
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if !h.service.IsOwner(user) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (h *OperatorHandler) Summary(w http.ResponseWriter, r *http.Request) {
	access, ok := h.readAccess(w, r, operator.ScopeSummary)
	if !ok {
		return
	}
	result, err := h.service.Summary(r.Context(), access)
	if err != nil {
		writeServerError(w, "operator summary failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *OperatorHandler) Activity(w http.ResponseWriter, r *http.Request) {
	access, ok := h.readAccess(w, r, operator.ScopeSummary)
	if !ok {
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		var err error
		days, err = strconv.Atoi(raw)
		if err != nil || (days != 30 && days != 90) {
			writeError(w, http.StatusBadRequest, "days must be 30 or 90")
			return
		}
	}
	result, err := h.service.Activity(r.Context(), access, days)
	if err != nil {
		writeServerError(w, "operator activity failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func operatorPageArgs(r *http.Request) (int64, int, error) {
	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		var err error
		after, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			return 0, 0, operator.ErrInvalidInput
		}
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, operator.ErrInvalidInput
		}
	}
	return after, limit, nil
}

func (h *OperatorHandler) Users(w http.ResponseWriter, r *http.Request) {
	access, ok := h.readAccess(w, r, operator.ScopeFull)
	if !ok {
		return
	}
	after, limit, err := operatorPageArgs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page parameters")
		return
	}
	result, err := h.service.Users(r.Context(), access, after, limit)
	if err != nil {
		writeServerError(w, "operator users failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *OperatorHandler) Households(w http.ResponseWriter, r *http.Request) {
	access, ok := h.readAccess(w, r, operator.ScopeFull)
	if !ok {
		return
	}
	after, limit, err := operatorPageArgs(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page parameters")
		return
	}
	result, err := h.service.Households(r.Context(), access, after, limit)
	if err != nil {
		writeServerError(w, "operator households failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *OperatorHandler) Keys(w http.ResponseWriter, r *http.Request) {
	if !h.ownerSession(w, r) {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	switch r.Method {
	case http.MethodGet:
		keys, err := h.service.ListKeys(r.Context(), user)
		if err != nil {
			writeServerError(w, "operator key list failed", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
	case http.MethodPost:
		var req struct {
			Name  string `json:"name"`
			Scope string `json:"scope"`
			Days  int    `json:"days"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid key request")
			return
		}
		key, token, err := h.service.CreateKey(r.Context(), user, req.Name, req.Scope, req.Days)
		if err != nil {
			switch {
			case errors.Is(err, operator.ErrInvalidInput):
				writeError(w, http.StatusBadRequest, err.Error())
			case errors.Is(err, operator.ErrRecentLogin), errors.Is(err, operator.ErrTooManyKeys):
				writeError(w, http.StatusForbidden, err.Error())
			case errors.Is(err, operator.ErrDenied):
				writeError(w, http.StatusForbidden, "forbidden")
			default:
				writeServerError(w, "operator key creation failed", err)
			}
			return
		}
		audit.Emit(r.Context(), h.logger, "operator.key_created", map[string]string{"key_id": key.ID, "scope": key.Scope})
		writeJSON(w, http.StatusCreated, map[string]any{"key": key, "token": token})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *OperatorHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	if !h.ownerSession(w, r) {
		return
	}
	user, _ := middleware.CurrentUser(r.Context())
	id := r.PathValue("id")
	if err := h.service.RevokeKey(r.Context(), user, id); err != nil {
		if errors.Is(err, operator.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid key ID")
			return
		}
		writeServerError(w, "operator key revocation failed", err)
		return
	}
	audit.Emit(r.Context(), h.logger, "operator.key_revoked", map[string]string{"key_id": id})
	w.WriteHeader(http.StatusNoContent)
}
