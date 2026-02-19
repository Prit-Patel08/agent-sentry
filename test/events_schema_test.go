package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowforge/internal/database"
)

func setupTempDB(t *testing.T) {
	t.Helper()
	oldPath, hadPath := os.LookupEnv("FLOWFORGE_DB_PATH")
	dbPath := filepath.Join(t.TempDir(), "flowforge-test.db")
	if err := os.Setenv("FLOWFORGE_DB_PATH", dbPath); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if err := database.InitDB(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		database.CloseDB()
		if hadPath {
			_ = os.Setenv("FLOWFORGE_DB_PATH", oldPath)
		} else {
			_ = os.Unsetenv("FLOWFORGE_DB_PATH")
		}
	})
}

func TestIncidentTimelineQueryReturnsChronologicalEvents(t *testing.T) {
	setupTempDB(t)
	database.SetRunID("run-test")
	incidentID := "incident-abc"

	_, err := database.InsertEvent(
		"cpu_spike",
		"system",
		"CPU exceeded 90% for 30s",
		"run-test",
		incidentID,
		"CPU spike",
		"high cpu",
		42,
		97.5,
		0.1,
		93.2,
	)
	if err != nil {
		t.Fatalf("insert event1: %v", err)
	}

	_, err = database.InsertEvent(
		"process_killed",
		"system",
		"Policy action executed",
		"run-test",
		incidentID,
		"Process killed",
		"killed process",
		42,
		98.0,
		0.1,
		95.0,
	)
	if err != nil {
		t.Fatalf("insert event2: %v", err)
	}

	timeline, err := database.GetIncidentTimelineByIncidentID(incidentID, 10)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(timeline))
	}
	if timeline[0].Type != "cpu_spike" {
		t.Fatalf("expected first event type cpu_spike, got %q", timeline[0].Type)
	}
	if timeline[1].Type != "process_killed" {
		t.Fatalf("expected second event type process_killed, got %q", timeline[1].Type)
	}
}

func TestEventsTableIsAppendOnly(t *testing.T) {
	setupTempDB(t)

	eventID, err := database.InsertEvent(
		"policy_dry_run",
		"system",
		"Dry run event",
		"run-test",
		"incident-append-only",
		"Dry run",
		"log only",
		0,
		0,
		0,
		50.0,
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	_, err = database.GetDB().Exec("UPDATE events SET title = 'mutated' WHERE event_id = ?", eventID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only update error, got: %v", err)
	}

	_, err = database.GetDB().Exec("DELETE FROM events WHERE event_id = ?", eventID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only delete error, got: %v", err)
	}
}
