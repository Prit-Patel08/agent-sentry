package database

import (
	"fmt"
	"testing"
	"time"
)

func TestControlPlaneReplayInsertGetTouch(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	err := InsertControlPlaneReplay(
		"idem-key-1",
		"POST /process/restart",
		"hash-1",
		202,
		`{"status":"restart_requested","pid":0}`,
	)
	if err != nil {
		t.Fatalf("InsertControlPlaneReplay: %v", err)
	}

	rec, err := GetControlPlaneReplay("idem-key-1", "POST /process/restart")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay: %v", err)
	}
	if rec.ResponseStatus != 202 {
		t.Fatalf("expected response_status=202, got %d", rec.ResponseStatus)
	}
	if rec.RequestHash != "hash-1" {
		t.Fatalf("expected request_hash=hash-1, got %q", rec.RequestHash)
	}
	if rec.ReplayCount != 0 {
		t.Fatalf("expected replay_count=0, got %d", rec.ReplayCount)
	}

	if err := TouchControlPlaneReplay(rec.ID); err != nil {
		t.Fatalf("TouchControlPlaneReplay: %v", err)
	}
	updated, err := GetControlPlaneReplay("idem-key-1", "POST /process/restart")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay(updated): %v", err)
	}
	if updated.ReplayCount != 1 {
		t.Fatalf("expected replay_count=1 after touch, got %d", updated.ReplayCount)
	}
}

func TestControlPlaneReplayInsertDoesNotOverwriteFirstResponse(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	if err := InsertControlPlaneReplay("idem-key-2", "POST /process/kill", "hash-original", 202, `{"status":"stop_requested"}`); err != nil {
		t.Fatalf("insert first replay row: %v", err)
	}
	if err := InsertControlPlaneReplay("idem-key-2", "POST /process/kill", "hash-new", 409, `{"error":"different"}`); err != nil {
		t.Fatalf("insert second replay row: %v", err)
	}

	rec, err := GetControlPlaneReplay("idem-key-2", "POST /process/kill")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay: %v", err)
	}
	if rec.RequestHash != "hash-original" {
		t.Fatalf("expected original request_hash preserved, got %q", rec.RequestHash)
	}
	if rec.ResponseStatus != 202 {
		t.Fatalf("expected original response_status preserved (202), got %d", rec.ResponseStatus)
	}
	if rec.ResponseBody != `{"status":"stop_requested"}` {
		t.Fatalf("expected original response_body preserved, got %q", rec.ResponseBody)
	}
}

func TestPurgeControlPlaneReplaysByRetentionDays(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("retention-key-%d", i)
		if err := InsertControlPlaneReplay(key, "POST /process/kill", fmt.Sprintf("hash-%d", i), 202, `{"status":"stop_requested"}`); err != nil {
			t.Fatalf("insert replay row %d: %v", i, err)
		}
	}

	if _, err := db.Exec(`
UPDATE control_plane_replays
SET last_seen_at = datetime('now', '-10 day')
WHERE idempotency_key = 'retention-key-0'
`); err != nil {
		t.Fatalf("age row for retention test: %v", err)
	}

	deleted, err := PurgeControlPlaneReplays(7, 0)
	if err != nil {
		t.Fatalf("PurgeControlPlaneReplays(retention): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected exactly 1 row deleted by retention, got %d", deleted)
	}

	count, err := CountControlPlaneReplayRows()
	if err != nil {
		t.Fatalf("CountControlPlaneReplayRows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 remaining rows after retention purge, got %d", count)
	}
}

func TestPurgeControlPlaneReplaysByMaxRows(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("maxrows-key-%d", i)
		if err := InsertControlPlaneReplay(key, "POST /process/restart", fmt.Sprintf("hash-%d", i), 202, `{"status":"restart_requested"}`); err != nil {
			t.Fatalf("insert replay row %d: %v", i, err)
		}
	}

	deleted, err := PurgeControlPlaneReplays(0, 2)
	if err != nil {
		t.Fatalf("PurgeControlPlaneReplays(maxRows): %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 rows deleted by maxRows trim, got %d", deleted)
	}

	count, err := CountControlPlaneReplayRows()
	if err != nil {
		t.Fatalf("CountControlPlaneReplayRows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 remaining rows after maxRows purge, got %d", count)
	}

	rows, err := ListControlPlaneReplays(10)
	if err != nil {
		t.Fatalf("ListControlPlaneReplays: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected list length 2 after maxRows purge, got %d", len(rows))
	}
	if rows[0].IdempotencyKey != "maxrows-key-4" || rows[1].IdempotencyKey != "maxrows-key-3" {
		t.Fatalf("expected newest keys preserved (4,3), got (%s,%s)", rows[0].IdempotencyKey, rows[1].IdempotencyKey)
	}
}

func TestGetControlPlaneReplayStats(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	if err := InsertControlPlaneReplay("stats-key-old", "POST /process/restart", "stats-hash-old", 202, `{"status":"restart_requested"}`); err != nil {
		t.Fatalf("insert old stats replay row: %v", err)
	}
	if err := InsertControlPlaneReplay("stats-key-new", "POST /process/kill", "stats-hash-new", 202, `{"status":"stop_requested"}`); err != nil {
		t.Fatalf("insert new stats replay row: %v", err)
	}
	if _, err := db.Exec(`
UPDATE control_plane_replays
SET last_seen_at = datetime('now', '-5 day')
WHERE idempotency_key = 'stats-key-old'
`); err != nil {
		t.Fatalf("age old row: %v", err)
	}
	if _, err := db.Exec(`
UPDATE control_plane_replays
SET last_seen_at = datetime('now')
WHERE idempotency_key = 'stats-key-new'
`); err != nil {
		t.Fatalf("freshen new row: %v", err)
	}

	stats, err := GetControlPlaneReplayStats()
	if err != nil {
		t.Fatalf("GetControlPlaneReplayStats: %v", err)
	}
	if stats.RowCount != 2 {
		t.Fatalf("expected row_count=2, got %d", stats.RowCount)
	}
	if stats.OldestAgeSeconds < 4*24*60*60 {
		t.Fatalf("expected oldest_age_seconds to reflect aged row, got %d", stats.OldestAgeSeconds)
	}
	if stats.NewestAgeSeconds > 120 {
		t.Fatalf("expected newest_age_seconds to be near-now, got %d", stats.NewestAgeSeconds)
	}
}

func TestGetControlPlaneReplayDailyTrend(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := InsertEvent(
			"audit",
			"control-plane",
			"served cached response",
			"run-replay-trend",
			"incident-replay-trend",
			"IDEMPOTENT_REPLAY",
			"served cached control-plane mutation response",
			0,
			0,
			0,
			0,
		); err != nil {
			t.Fatalf("insert replay event: %v", err)
		}
	}
	if _, err := InsertEvent(
		"audit",
		"control-plane",
		"idempotency key reused with different payload",
		"run-replay-trend",
		"incident-replay-trend",
		"IDEMPOTENT_CONFLICT",
		"idempotency key reused with different request payload",
		0,
		0,
		0,
		0,
	); err != nil {
		t.Fatalf("insert conflict event: %v", err)
	}

	points, err := GetControlPlaneReplayDailyTrend(3)
	if err != nil {
		t.Fatalf("GetControlPlaneReplayDailyTrend: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 daily points, got %d", len(points))
	}

	today := time.Now().UTC().Format("2006-01-02")
	foundToday := false
	for _, point := range points {
		if point.Day == "" {
			t.Fatalf("expected non-empty day in point: %#v", point)
		}
		if point.Day != today {
			continue
		}
		foundToday = true
		if point.ReplayEvents != 2 {
			t.Fatalf("expected today replay_events=2, got %d", point.ReplayEvents)
		}
		if point.ConflictEvents != 1 {
			t.Fatalf("expected today conflict_events=1, got %d", point.ConflictEvents)
		}
	}
	if !foundToday {
		t.Fatalf("expected trend to include today (%s)", today)
	}
}
