package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// TelemetryEvent matches the CLI's Event struct.
type TelemetryEvent struct {
	EventName  string           `json:"event"`
	Timestamp  string           `json:"ts"`
	InstallID  string           `json:"install_id"`
	Version    string           `json:"version"`
	OS         string           `json:"os"`
	Arch       string           `json:"arch"`
	Command    string           `json:"command"`
	DurMs      int64            `json:"dur_ms"`
	Stats      map[string]int64 `json:"stats,omitempty"`
	Result     string           `json:"result"`
	ErrorClass string           `json:"error_class,omitempty"`
}

// IngestTelemetry receives a batch of anonymous telemetry events from the CLI.
// Events are gzipped JSONL (one JSON object per line).
func (h *Handler) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
	// Limit body size (1 MB compressed)
	limitReader := io.LimitReader(r.Body, 1<<20)

	var reader io.Reader
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(limitReader)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gzip", err)
			return
		}
		defer gz.Close()
		reader = gz
	} else {
		reader = limitReader
	}

	// Parse JSONL
	decoder := json.NewDecoder(reader)
	var events []TelemetryEvent
	for decoder.More() {
		var ev TelemetryEvent
		if err := decoder.Decode(&ev); err != nil {
			continue // skip malformed lines
		}
		if ev.InstallID == "" || ev.EventName == "" {
			continue
		}
		events = append(events, ev)
	}

	if len(events) == 0 {
		writeJSON(w, http.StatusOK, map[string]int{"accepted": 0})
		return
	}

	// Store events
	if err := h.db.InsertTelemetryEvents(events); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store events", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"accepted": len(events)})
}
