package database

import (
	"database/sql"
	"fmt"
	"strings"
)

type ControlPlaneReplay struct {
	ID             int    `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Endpoint       string `json:"endpoint"`
	RequestHash    string `json:"request_hash"`
	ResponseStatus int    `json:"response_status"`
	ResponseBody   string `json:"response_body"`
	ReplayCount    int    `json:"replay_count"`
	CreatedAt      string `json:"created_at"`
	LastSeenAt     string `json:"last_seen_at"`
}

func GetControlPlaneReplay(idempotencyKey, endpoint string) (ControlPlaneReplay, error) {
	if db == nil {
		return ControlPlaneReplay{}, fmt.Errorf("db not initialized")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	endpoint = strings.TrimSpace(endpoint)
	if idempotencyKey == "" || endpoint == "" {
		return ControlPlaneReplay{}, fmt.Errorf("idempotency key and endpoint are required")
	}

	var rec ControlPlaneReplay
	err := db.QueryRow(`
SELECT
	id,
	idempotency_key,
	endpoint,
	request_hash,
	response_status,
	response_body,
	replay_count,
	COALESCE(created_at, CURRENT_TIMESTAMP),
	COALESCE(last_seen_at, CURRENT_TIMESTAMP)
FROM control_plane_replays
WHERE idempotency_key = ? AND endpoint = ?
`, idempotencyKey, endpoint).Scan(
		&rec.ID,
		&rec.IdempotencyKey,
		&rec.Endpoint,
		&rec.RequestHash,
		&rec.ResponseStatus,
		&rec.ResponseBody,
		&rec.ReplayCount,
		&rec.CreatedAt,
		&rec.LastSeenAt,
	)
	if err != nil {
		return ControlPlaneReplay{}, err
	}
	return rec, nil
}

func InsertControlPlaneReplay(idempotencyKey, endpoint, requestHash string, responseStatus int, responseBody string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	endpoint = strings.TrimSpace(endpoint)
	requestHash = strings.TrimSpace(requestHash)
	if idempotencyKey == "" || endpoint == "" || requestHash == "" {
		return fmt.Errorf("idempotency key, endpoint, and request hash are required")
	}
	if responseStatus <= 0 {
		return fmt.Errorf("response status must be > 0")
	}
	if strings.TrimSpace(responseBody) == "" {
		responseBody = "{}"
	}

	_, err := db.Exec(`
INSERT INTO control_plane_replays(
	idempotency_key,
	endpoint,
	request_hash,
	response_status,
	response_body,
	replay_count,
	created_at,
	last_seen_at
) VALUES(?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(idempotency_key, endpoint) DO NOTHING
`, idempotencyKey, endpoint, requestHash, responseStatus, responseBody)
	return err
}

func TouchControlPlaneReplay(id int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	if id <= 0 {
		return fmt.Errorf("id must be > 0")
	}

	_, err := db.Exec(`
UPDATE control_plane_replays
SET replay_count = replay_count + 1,
	last_seen_at = CURRENT_TIMESTAMP
WHERE id = ?
`, id)
	return err
}

func ListControlPlaneReplays(limit int) ([]ControlPlaneReplay, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.Query(`
SELECT
	id,
	idempotency_key,
	endpoint,
	request_hash,
	response_status,
	response_body,
	replay_count,
	COALESCE(created_at, CURRENT_TIMESTAMP),
	COALESCE(last_seen_at, CURRENT_TIMESTAMP)
FROM control_plane_replays
ORDER BY last_seen_at DESC, id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ControlPlaneReplay, 0, limit)
	for rows.Next() {
		var rec ControlPlaneReplay
		if err := rows.Scan(
			&rec.ID,
			&rec.IdempotencyKey,
			&rec.Endpoint,
			&rec.RequestHash,
			&rec.ResponseStatus,
			&rec.ResponseBody,
			&rec.ReplayCount,
			&rec.CreatedAt,
			&rec.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func CountControlPlaneReplayRows() (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM control_plane_replays").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}
