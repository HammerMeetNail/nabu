package handlers

import (
	"net/http"
	"strconv"

	"github.com/dave/choresy/internal/chore"
	"github.com/dave/choresy/internal/middleware"
)

type ChoreHandler struct {
	service *chore.Service
}

func NewChoreHandler(service *chore.Service) *ChoreHandler {
	return &ChoreHandler{service: service}
}

func (h *ChoreHandler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	chores, err := h.service.ListChores(r.Context(), *user.HouseholdID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if chores == nil {
		chores = []chore.Chore{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"chores": chores})
}

func (h *ChoreHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		Category string `json:"category"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.service.CreateChore(r.Context(), *user.HouseholdID, user.ID, req.Name, req.Icon, req.Color, req.Category)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"chore": created})
}

func (h *ChoreHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	c, err := h.service.GetChore(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "chore not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chore": c})
}

func (h *ChoreHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, _ = middleware.CurrentUser(r.Context())

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		Category string `json:"category"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateChore(r.Context(), id, req.Name, req.Icon, req.Color, req.Category); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ChoreHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chore id")
		return
	}

	if err := h.service.DeleteChore(r.Context(), id); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *ChoreHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	var req struct {
		ChoreIDs []int64 `json:"choreIds"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.ReorderChores(r.Context(), *user.HouseholdID, req.ChoreIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

func (h *ChoreHandler) GetDefaults(w http.ResponseWriter, r *http.Request) {
	defaults := h.service.GetSystemDefaults()
	writeJSON(w, http.StatusOK, map[string]any{"defaults": defaults})
}

func (h *ChoreHandler) SeedDefaults(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.CurrentUser(r.Context())
	if user.HouseholdID == nil {
		writeError(w, http.StatusUnauthorized, "no household")
		return
	}

	if err := h.service.SeedDefaultChores(r.Context(), *user.HouseholdID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
}
