package database

import (
	"agent-sentry/internal/encryption"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type Incident struct {
	ID                   int     `json:"id"`
	Timestamp            string  `json:"timestamp"`
	Command              string  `json:"command"`
	ModelName            string  `json:"model_name"`
	ExitReason           string  `json:"exit_reason"`
	MaxCPU               float64 `json:"max_cpu"`
	Pattern              string  `json:"pattern"`
	TokenSavingsEstimate float64 `json:"token_savings_estimate"`
	TokenCount           int     `json:"token_count"`
	Cost                 float64 `json:"cost"`
	AgentID              string  `json:"agent_uuid"`
	AgentVersion         string  `json:"agent_version"`
}

func InitDB() error {
	dbPath := os.Getenv("SENTRY_DB_PATH")
	if dbPath == "" {
		dbPath = "sentry.db"
	}
	var err error

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		command TEXT,
		model_name TEXT,
		exit_reason TEXT,
		max_cpu REAL,
		pattern TEXT,
		token_savings_estimate REAL
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		return err
	}

	// Migrations
	db.Exec("ALTER TABLE incidents ADD COLUMN token_count INTEGER DEFAULT 0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN cost REAL DEFAULT 0.0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN agent_id TEXT DEFAULT '';")
	db.Exec("ALTER TABLE incidents ADD COLUMN agent_version TEXT DEFAULT '';")

	return nil
}

func GetDB() *sql.DB {
	return db
}

func CloseDB() {
	if db != nil {
		db.Close()
	}
}

func LogIncident(command, modelName, exitReason string, maxCpu float64, pattern string, savings float64, tokenCount int, cost float64, agentID, agentVersion string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	// Encrypt sensitive fields
	encCmd, _ := encryption.Encrypt(command)
	encPat, _ := encryption.Encrypt(pattern)

	// Fallback to raw if encryption returns empty string
	if encCmd == "" {
		encCmd = command
	}
	if encPat == "" {
		encPat = pattern
	}

	stmt, err := db.Prepare("INSERT INTO incidents(command, model_name, exit_reason, max_cpu, pattern, token_savings_estimate, token_count, cost, agent_id, agent_version) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(encCmd, modelName, exitReason, maxCpu, encPat, savings, tokenCount, cost, agentID, agentVersion)
	return err
}

func GetIncidentByID(id int) (Incident, error) {
	var i Incident
	if db == nil {
		return i, fmt.Errorf("db missing")
	}

	row := db.QueryRow("SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, '') FROM incidents WHERE id = ?", id)
	err := row.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion)

	if err == nil {
		if dec, e := encryption.Decrypt(i.Command); e == nil {
			i.Command = dec
		}
		if dec, e := encryption.Decrypt(i.Pattern); e == nil {
			i.Pattern = dec
		}
	}
	return i, err
}

func GetAllIncidents() ([]Incident, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}

	rows, err := db.Query("SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, '') FROM incidents ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Incident
	for rows.Next() {
		var i Incident
		if err := rows.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion); err != nil {
			return nil, err
		}
		if dec, e := encryption.Decrypt(i.Command); e == nil {
			i.Command = dec
		}
		if dec, e := encryption.Decrypt(i.Pattern); e == nil {
			i.Pattern = dec
		}
		list = append(list, i)
	}
	return list, nil
}

func PruneIncidents(days int) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db missing")
	}

	result, err := db.Exec("DELETE FROM incidents WHERE timestamp < datetime('now', ?)", fmt.Sprintf("-%d days", days))
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()

	// Optimize DB to reclaim space
	_, err = db.Exec("VACUUM")
	if err != nil {
		return rowsAffected, fmt.Errorf("vacuum failed: %v", err)
	}

	return rowsAffected, nil
}
