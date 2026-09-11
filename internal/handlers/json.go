package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/HammerMeetNail/nabu/internal/diagnostics"
	"io"
	"log"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// The static message identifies the operation. Never format err itself:
// transport and database errors may contain secrets or private payloads.
func writeServerError(w http.ResponseWriter, message string, err error) {
	id := diagnoseError(w, message, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": message, "requestId": id})
}

func diagnoseError(w http.ResponseWriter, message string, err error) string {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = diagnostics.RequestID()
		w.Header().Set("X-Request-ID", id)
	}
	log.Printf("operation=%q error_class=%s request_id=%s", message, diagnostics.ErrorClass(err), id)
	return id
}

func readJSON(r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB
	return json.NewDecoder(r.Body).Decode(target)
}

// readPatchJSON retains field presence. JSON null is a deliberate clear for
// nullable fields; absence leaves the stored value unchanged.
func readPatchJSON(r *http.Request, target any) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	source := json.NewDecoder(r.Body)
	if err := source.Decode(&fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("expected an object")
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return nil, err
	}
	// Consume no extra JSON document (the endpoint accepts one PATCH only).
	var extra any
	if err := source.Decode(&extra); err != io.EOF {
		return nil, errors.New("extra request data")
	}
	return fields, nil
}
