package database

import (
	"testing"
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
