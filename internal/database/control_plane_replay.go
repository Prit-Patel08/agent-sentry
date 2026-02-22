package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
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

type ControlPlaneReplayStats struct {
	RowCount         int `json:"row_count"`
	OldestAgeSeconds int `json:"oldest_age_seconds"`
	NewestAgeSeconds int `json:"newest_age_seconds"`
}

type ControlPlaneReplayDailyTrend struct {
	Day            string `json:"day"`
	ReplayEvents   int    `json:"replay_events"`
	ConflictEvents int    `json:"conflict_events"`
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

func GetControlPlaneReplayStats() (ControlPlaneReplayStats, error) {
	if db == nil {
		return ControlPlaneReplayStats{}, fmt.Errorf("db not initialized")
	}
	var stats ControlPlaneReplayStats
	if err := db.QueryRow(`
SELECT
	COUNT(*) AS row_count,
	COALESCE(CAST((julianday('now') - julianday(MIN(last_seen_at))) * 86400 AS INTEGER), 0) AS oldest_age_seconds,
	COALESCE(CAST((julianday('now') - julianday(MAX(last_seen_at))) * 86400 AS INTEGER), 0) AS newest_age_seconds
FROM control_plane_replays
`).Scan(
		&stats.RowCount,
		&stats.OldestAgeSeconds,
		&stats.NewestAgeSeconds,
	); err != nil {
		return ControlPlaneReplayStats{}, err
	}
	return stats, nil
}

func GetControlPlaneReplayDailyTrend(days int) ([]ControlPlaneReplayDailyTrend, error) {
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}

	timeCol := "created_at"
	hasCreatedAt, err := columnExists("events", "created_at")
	if err != nil {
		return nil, err
	}
	if !hasCreatedAt {
		timeCol = "timestamp"
	}

	window := fmt.Sprintf("-%d day", days-1)
	query := fmt.Sprintf(`
SELECT
	date(COALESCE(%[1]s, timestamp, CURRENT_TIMESTAMP)) AS day,
	SUM(CASE WHEN UPPER(title)='IDEMPOTENT_REPLAY' THEN 1 ELSE 0 END) AS replay_events,
	SUM(CASE WHEN UPPER(title)='IDEMPOTENT_CONFLICT' THEN 1 ELSE 0 END) AS conflict_events
FROM events
WHERE event_type='audit'
  AND UPPER(title) IN ('IDEMPOTENT_REPLAY', 'IDEMPOTENT_CONFLICT')
  AND date(COALESCE(%[1]s, timestamp, CURRENT_TIMESTAMP)) >= date('now', ?)
GROUP BY day
ORDER BY day ASC
`, timeCol)

	rows, err := db.Query(query, window)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byDay := make(map[string]ControlPlaneReplayDailyTrend, days)
	for rows.Next() {
		var point ControlPlaneReplayDailyTrend
		if err := rows.Scan(&point.Day, &point.ReplayEvents, &point.ConflictEvents); err != nil {
			return nil, err
		}
		byDay[point.Day] = point
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	startDay := time.Now().UTC().AddDate(0, 0, -(days - 1))
	out := make([]ControlPlaneReplayDailyTrend, 0, days)
	for i := 0; i < days; i++ {
		day := startDay.AddDate(0, 0, i).Format("2006-01-02")
		point, ok := byDay[day]
		if !ok {
			point = ControlPlaneReplayDailyTrend{Day: day}
		}
		out = append(out, point)
	}
	return out, nil
}

// PurgeControlPlaneReplays deletes stale replay rows.
//
// retentionDays:
//
//	> 0 => delete rows where last_seen_at is older than retentionDays
//	<= 0 => skip age-based deletion
//
// maxRows:
//
//	> 0 => keep only newest maxRows by (last_seen_at DESC, id DESC)
//	<= 0 => skip count-based trimming
func PurgeControlPlaneReplays(retentionDays, maxRows int) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	if retentionDays <= 0 && maxRows <= 0 {
		return 0, nil
	}

	deletedTotal := 0

	if retentionDays > 0 {
		window := fmt.Sprintf("-%d day", retentionDays)
		res, err := db.Exec(`
DELETE FROM control_plane_replays
WHERE last_seen_at < datetime('now', ?)
`, window)
		if err != nil {
			return deletedTotal, err
		}
		if n, err := res.RowsAffected(); err == nil {
			deletedTotal += int(n)
		}
	}

	if maxRows > 0 {
		res, err := db.Exec(`
DELETE FROM control_plane_replays
WHERE id IN (
  SELECT id
  FROM control_plane_replays
  ORDER BY last_seen_at DESC, id DESC
  LIMIT -1 OFFSET ?
)
`, maxRows)
		if err != nil {
			return deletedTotal, err
		}
		if n, err := res.RowsAffected(); err == nil {
			deletedTotal += int(n)
		}
	}

	return deletedTotal, nil
}

func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}
