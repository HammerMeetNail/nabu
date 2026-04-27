package handlers

import "net/http"

type HealthResponse struct {
	Status string `json:"status"`
}

func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func Ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ready"})
}
