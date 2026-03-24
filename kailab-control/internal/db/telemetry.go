package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TelemetryEvent matches the API struct for insertion.
type TelemetryEvent struct {
	EventName  string           `json:"event"`
	Timestamp  string           `json:"ts"`
	InstallID  string           `json:"install_id"`
	Version    string           `json:"version"`
	OS         string           `json:"os"`
	Arch       string           `json:"arch"`
	Command    string           `json:"command"`
	DurMs      int64            `json:"dur_ms"`
	Result     string           `json:"result"`
	ErrorClass string           `json:"error_class,omitempty"`
	Stats      map[string]int64 `json:"stats,omitempty"`
}

// InsertTelemetryEvents batch-inserts telemetry events.
func (db *DB) InsertTelemetryEvents(events interface{}) error {
	// Type-assert from the API layer's type
	type apiEvent struct {
		EventName  string           `json:"event"`
		Timestamp  string           `json:"ts"`
		InstallID  string           `json:"install_id"`
		Version    string           `json:"version"`
		OS         string           `json:"os"`
		Arch       string           `json:"arch"`
		Command    string           `json:"command"`
		DurMs      int64            `json:"dur_ms"`
		Result     string           `json:"result"`
		ErrorClass string           `json:"error_class,omitempty"`
		Stats      map[string]int64 `json:"stats,omitempty"`
	}

	// Marshal/unmarshal to convert types (both are the same shape)
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}
	var evs []apiEvent
	if err := json.Unmarshal(b, &evs); err != nil {
		return err
	}

	if len(evs) == 0 {
		return nil
	}

	now := time.Now().Unix()

	if db.driver == DriverPostgres {
		// Batch insert with VALUES
		var values []string
		var args []interface{}
		for i, ev := range evs {
			statsJSON := "{}"
			if ev.Stats != nil {
				b, _ := json.Marshal(ev.Stats)
				statsJSON = string(b)
			}
			base := i * 11
			values = append(values, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11))
			args = append(args, ev.EventName, ev.Timestamp, ev.InstallID, ev.Version,
				ev.OS, ev.Arch, ev.Command, ev.DurMs, ev.Result, ev.ErrorClass, statsJSON)
		}

		query := fmt.Sprintf(`INSERT INTO telemetry_events
			(event_name, timestamp, install_id, version, os, arch, command, dur_ms, result, error_class, stats)
			VALUES %s`, strings.Join(values, ","))

		_, err := db.DB.Exec(query, args...)
		return err
	}

	// SQLite fallback
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO telemetry_events
		(event_name, timestamp, install_id, version, os, arch, command, dur_ms, result, error_class, stats, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ev := range evs {
		statsJSON := "{}"
		if ev.Stats != nil {
			b, _ := json.Marshal(ev.Stats)
			statsJSON = string(b)
		}
		_, err := stmt.Exec(ev.EventName, ev.Timestamp, ev.InstallID, ev.Version,
			ev.OS, ev.Arch, ev.Command, ev.DurMs, ev.Result, ev.ErrorClass, statsJSON, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
