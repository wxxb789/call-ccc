// Package opencodefixture builds a throw-away OpenCode store for tests: the tables the inherited
// AgentsView OpenCode reader queries, filled from one import document of this repository's OpenCode
// writer.
//
// It exists because the only way this build materialises an OpenCode session is the harness's own
// importer, so a case that needs a *store* the reader can open -- the structural read-back, the CLI's
// report -- has two choices: run the real harness, or write the harness's own rows. This package is the
// second: a fixture, not a writer of this build's. The product writes no SQL into an OpenCode store, and
// a change that made it do so would make this package unnecessary rather than entitled to grow.
//
// It is deliberately outside the continuation core (the calling test packages keep their own assertions)
// and it carries no schema of its own invention: the three tables and their columns are exactly what
// internal/parser's OpenCode reader selects.
package opencodefixture

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	// The SQLite driver the inherited reader uses: the same one, so a fixture store is readable by the
	// code under test rather than by a driver that happens to agree.
	_ "github.com/mattn/go-sqlite3"
)

// StoreName is the database file name the harness keeps its sessions in.
const StoreName = "opencode.db"

// document is as much of an OpenCode import document as a store needs: the session row's identity and
// its messages. Every other member of the document is the writer's business, not the fixture's.
type document struct {
	Info struct {
		ID        string `json:"id"`
		ProjectID string `json:"projectID"`
		Location  struct {
			Directory string `json:"directory"`
		} `json:"location"`
		Time struct {
			Created int64 `json:"created"`
			Updated int64 `json:"updated"`
		} `json:"time"`
	} `json:"info"`
	// The messages are kept as the writer's own bytes: the store's rows hold the document's own members,
	// not a re-encoded copy of them.
	Messages []json.RawMessage `json:"messages"`
}

// Store writes one session document into `<dataDir>/opencode.db`, creating the directory and the file
// when they do not exist, and returns the store's path and the session's own id. A second call against
// the same directory adds another session to the same store.
func Store(dataDir string, encoded []byte) (string, string, error) {
	var document document
	if err := json.Unmarshal(encoded, &document); err != nil {
		return "", "", fmt.Errorf("the fixture document does not parse: %w", err)
	}
	if document.Info.ID == "" {
		return "", "", fmt.Errorf("the fixture document carries no session id")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", dataDir, err)
	}
	path := filepath.Join(dataDir, StoreName)
	database, err := sql.Open("sqlite3", path)
	if err != nil {
		return "", "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer database.Close()
	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS project (
			id TEXT PRIMARY KEY,
			worktree TEXT
		);
		CREATE TABLE IF NOT EXISTS session_v2 (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			parent_id TEXT,
			title TEXT,
			directory TEXT,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			time_idle INTEGER
		);
		CREATE TABLE IF NOT EXISTS session_message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			type TEXT NOT NULL,
			seq INTEGER NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
	`); err != nil {
		return "", "", fmt.Errorf("creating the store's tables: %w", err)
	}
	if _, err := database.Exec(`INSERT OR IGNORE INTO project (id, worktree) VALUES (?, ?)`,
		document.Info.ProjectID, document.Info.Location.Directory); err != nil {
		return "", "", fmt.Errorf("writing the project row: %w", err)
	}
	if _, err := database.Exec(`INSERT INTO session_v2
		(id, project_id, title, directory, time_created, time_updated) VALUES (?, ?, '', ?, ?, ?)`,
		document.Info.ID, document.Info.ProjectID, document.Info.Location.Directory,
		document.Info.Time.Created, document.Info.Time.Updated); err != nil {
		return "", "", fmt.Errorf("writing the session row: %w", err)
	}
	for seq, message := range document.Messages {
		var identity struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Time struct {
				Created int64 `json:"created"`
			} `json:"time"`
		}
		if err := json.Unmarshal(message, &identity); err != nil {
			return "", "", fmt.Errorf("message %d does not parse: %w", seq, err)
		}
		if _, err := database.Exec(`INSERT INTO session_message
			(id, session_id, type, seq, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			identity.ID, document.Info.ID, identity.Type, seq, identity.Time.Created, identity.Time.Created,
			string(message)); err != nil {
			return "", "", fmt.Errorf("writing message %d: %w", seq, err)
		}
	}
	return path, document.Info.ID, nil
}
