package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and provides CRUD methods for the app.
type DB struct {
	pool *pgxpool.Pool
}

// RunRow is a row from sf.runs.
type RunRow struct {
	ID          int64
	UserID      int64
	IdeaText    string
	Canvas      json.RawMessage // null if nil
	Landing     json.RawMessage
	Status      string
	Results     json.RawMessage
	Personas    json.RawMessage
	Validations json.RawMessage
	ErrorMsg    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// OpenDB opens the connection pool and applies any pending migrations.
func OpenDB(ctx context.Context, connStr string) (*DB, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	db := &DB{pool: pool}
	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return db, nil
}

// runMigrations reads embedded SQL files from migrationsFS and applies
// any that are not yet recorded in sf.schema_migrations.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure the migrations tracking table exists first.
	_, err := pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS sf;
		CREATE TABLE IF NOT EXISTS sf.schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	// Collect migration file names from the embedded FS.
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Load already-applied versions.
	rows, err := pool.Query(ctx, `SELECT version FROM sf.schema_migrations`)
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	for _, name := range files {
		if applied[name] {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		log.Printf("[db] applying migration %s", name)
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("exec %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO sf.schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`, name,
		); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
	}
	return nil
}

// ── Users ────────────────────────────────────────────────────────────────────

func (db *DB) CreateUser(ctx context.Context, email, passwordHash string) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx,
		`INSERT INTO sf.users(email, password_hash) VALUES($1, $2) RETURNING id`,
		email, passwordHash,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateUser: %w", err)
	}
	return id, nil
}

func (db *DB) GetUserByEmail(ctx context.Context, email string) (id int64, hash string, err error) {
	err = db.pool.QueryRow(ctx,
		`SELECT id, password_hash FROM sf.users WHERE email=$1`, email,
	).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", fmt.Errorf("user not found")
	}
	return id, hash, err
}

func (db *DB) GetUserByID(ctx context.Context, id int64) (email string, err error) {
	err = db.pool.QueryRow(ctx,
		`SELECT email FROM sf.users WHERE id=$1`, id,
	).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("user not found")
	}
	return email, err
}

// ── Sessions ─────────────────────────────────────────────────────────────────

func (db *DB) CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO sf.sessions(token, user_id, expires_at) VALUES($1, $2, $3)`,
		token, userID, expiresAt,
	)
	return err
}

// GetSessionUser returns the user_id for a valid (non-expired) session token.
func (db *DB) GetSessionUser(ctx context.Context, token string) (userID int64, err error) {
	err = db.pool.QueryRow(ctx,
		`SELECT user_id FROM sf.sessions WHERE token=$1 AND expires_at > now()`, token,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("session not found or expired")
	}
	return userID, err
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM sf.sessions WHERE token=$1`, token)
	return err
}

func (db *DB) CleanExpiredSessions(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM sf.sessions WHERE expires_at <= now()`)
	return err
}

// ── Runs ──────────────────────────────────────────────────────────────────────

func (db *DB) CreateRun(ctx context.Context, userID int64, ideaText string) (int64, error) {
	var id int64
	err := db.pool.QueryRow(ctx,
		`INSERT INTO sf.runs(user_id, idea_text) VALUES($1, $2) RETURNING id`,
		userID, ideaText,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("CreateRun: %w", err)
	}
	return id, nil
}

func (db *DB) GetRun(ctx context.Context, id, userID int64) (*RunRow, error) {
	row := &RunRow{}
	err := db.pool.QueryRow(ctx, `
		SELECT id, user_id, idea_text,
		       canvas, landing, status,
		       results, personas, validations,
		       COALESCE(error_message,''), created_at, updated_at
		FROM sf.runs
		WHERE id=$1 AND user_id=$2
	`, id, userID).Scan(
		&row.ID, &row.UserID, &row.IdeaText,
		&row.Canvas, &row.Landing, &row.Status,
		&row.Results, &row.Personas, &row.Validations,
		&row.ErrorMsg, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetRun: %w", err)
	}
	return row, nil
}

func (db *DB) ListRuns(ctx context.Context, userID int64) ([]RunRow, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, user_id, idea_text,
		       canvas, landing, status,
		       results, personas, validations,
		       COALESCE(error_message,''), created_at, updated_at
		FROM sf.runs
		WHERE user_id=$1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("ListRuns: %w", err)
	}
	defer rows.Close()

	var out []RunRow
	for rows.Next() {
		var r RunRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.IdeaText,
			&r.Canvas, &r.Landing, &r.Status,
			&r.Results, &r.Personas, &r.Validations,
			&r.ErrorMsg, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) UpdateRunCanvas(ctx context.Context, id int64, canvas json.RawMessage) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE sf.runs SET canvas=$1, updated_at=now() WHERE id=$2`,
		canvas, id,
	)
	return err
}

func (db *DB) UpdateRunLanding(ctx context.Context, id int64, landing json.RawMessage) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE sf.runs SET landing=$1, updated_at=now() WHERE id=$2`,
		landing, id,
	)
	return err
}

func (db *DB) UpdateRunStatus(ctx context.Context, id int64, status string) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE sf.runs SET status=$1, updated_at=now() WHERE id=$2`,
		status, id,
	)
	return err
}

func (db *DB) UpdateRunResults(ctx context.Context, id int64, personas, validations, results json.RawMessage, status string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE sf.runs
		SET personas=$1, validations=$2, results=$3, status=$4, updated_at=now()
		WHERE id=$5
	`, personas, validations, results, status, id)
	return err
}
