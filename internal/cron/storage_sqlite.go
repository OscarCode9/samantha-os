package cron

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStorage implementa Storage usando SQLite.
type SQLiteStorage struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteStorage crea una instancia de SQLiteStorage.
// dbPath puede ser ":memory:" para una base de datos en memoria o una ruta a un archivo.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configurar la conexión.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Crear tablas si no existen.
	if err := initializeSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return &SQLiteStorage{db: db}, nil
}

// SaveJob persiste un job.
func (s *SQLiteStorage) SaveJob(ctx context.Context, job *CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payloadBytes, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	scheduleBytes, err := json.Marshal(job.Schedule)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}

	query := `
	INSERT INTO cron_jobs (
		id, name, schedule, payload, enabled, delivery_mode, webhook_url, webhook_secret,
		created_at, updated_at, last_run_at, last_run_status, last_error, next_run_at,
		run_count, consecutive_failures, max_retries
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name, schedule=excluded.schedule, payload=excluded.payload,
		enabled=excluded.enabled, delivery_mode=excluded.delivery_mode,
		webhook_url=excluded.webhook_url, webhook_secret=excluded.webhook_secret,
		updated_at=excluded.updated_at, last_run_at=excluded.last_run_at,
		last_run_status=excluded.last_run_status, last_error=excluded.last_error,
		next_run_at=excluded.next_run_at, run_count=excluded.run_count,
		consecutive_failures=excluded.consecutive_failures, max_retries=excluded.max_retries
	`

	_, err = s.db.ExecContext(ctx, query,
		job.ID, job.Name, string(scheduleBytes), string(payloadBytes), job.Enabled,
		job.DeliveryMode, job.WebhookURL, job.WebhookSecret,
		job.CreatedAt, job.UpdatedAt, job.LastRunAt, job.LastRunStatus, job.LastError, job.NextRunAt,
		job.RunCount, job.ConsecutiveFailures, job.MaxRetries,
	)
	return err
}

// LoadAllJobs carga todos los jobs de la base de datos.
func (s *SQLiteStorage) LoadAllJobs(ctx context.Context) ([]*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT
		id, name, schedule, payload, enabled, delivery_mode, webhook_url, webhook_secret,
		created_at, updated_at, last_run_at, last_run_status, last_error, next_run_at,
		run_count, consecutive_failures, max_retries
	FROM cron_jobs
	ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*CronJob
	for rows.Next() {
		job := &CronJob{}
		var schedule, payload string

		err := rows.Scan(
			&job.ID, &job.Name, &schedule, &payload, &job.Enabled,
			&job.DeliveryMode, &job.WebhookURL, &job.WebhookSecret,
			&job.CreatedAt, &job.UpdatedAt, &job.LastRunAt, &job.LastRunStatus, &job.LastError, &job.NextRunAt,
			&job.RunCount, &job.ConsecutiveFailures, &job.MaxRetries,
		)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}

		if err := json.Unmarshal([]byte(schedule), &job.Schedule); err != nil {
			return nil, fmt.Errorf("unmarshal schedule: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &job.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// GetJob carga un job por su ID.
func (s *SQLiteStorage) GetJob(ctx context.Context, jobID string) (*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT
		id, name, schedule, payload, enabled, delivery_mode, webhook_url, webhook_secret,
		created_at, updated_at, last_run_at, last_run_status, last_error, next_run_at,
		run_count, consecutive_failures, max_retries
	FROM cron_jobs
	WHERE id = ?
	`

	job := &CronJob{}
	var schedule, payload string

	err := s.db.QueryRowContext(ctx, query, jobID).Scan(
		&job.ID, &job.Name, &schedule, &payload, &job.Enabled,
		&job.DeliveryMode, &job.WebhookURL, &job.WebhookSecret,
		&job.CreatedAt, &job.UpdatedAt, &job.LastRunAt, &job.LastRunStatus, &job.LastError, &job.NextRunAt,
		&job.RunCount, &job.ConsecutiveFailures, &job.MaxRetries,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query job: %w", err)
	}

	if err := json.Unmarshal([]byte(schedule), &job.Schedule); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &job.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	return job, nil
}

// DeleteJob elimina un job.
func (s *SQLiteStorage) DeleteJob(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM cron_jobs WHERE id = ?", jobID)
	return err
}

// SaveRunLog guarda un log de ejecución.
func (s *SQLiteStorage) SaveRunLog(ctx context.Context, log *JobRunLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	INSERT INTO cron_run_logs (id, job_id, started_at, completed_at, status, error, output, delivery_mode, delivered_to)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		log.ID, log.JobID, log.StartedAt, log.CompletedAt, log.Status, log.Error, log.Output, log.DeliveryMode, log.DeliveredTo,
	)
	return err
}

// ListRunLogs lista los logs de ejecución de un job.
func (s *SQLiteStorage) ListRunLogs(ctx context.Context, jobID string, limit int) ([]*JobRunLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
	SELECT id, job_id, started_at, completed_at, status, error, output, delivery_mode, delivered_to
	FROM cron_run_logs
	WHERE job_id = ?
	ORDER BY started_at DESC
	LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, jobID, limit)
	if err != nil {
		return nil, fmt.Errorf("query run logs: %w", err)
	}
	defer rows.Close()

	var logs []*JobRunLog
	for rows.Next() {
		log := &JobRunLog{}
		if err := rows.Scan(
			&log.ID, &log.JobID, &log.StartedAt, &log.CompletedAt, &log.Status, &log.Error, &log.Output, &log.DeliveryMode, &log.DeliveredTo,
		); err != nil {
			return nil, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, log)
	}

	return logs, rows.Err()
}

// Close cierra la conexión a la base de datos.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// initializeSchema crea las tablas necesarias si no existen.
func initializeSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS cron_jobs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		schedule TEXT NOT NULL,
		payload TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		delivery_mode TEXT NOT NULL DEFAULT 'none',
		webhook_url TEXT,
		webhook_secret TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_run_at TIMESTAMP,
		last_run_status TEXT,
		last_error TEXT,
		next_run_at TIMESTAMP,
		run_count INTEGER NOT NULL DEFAULT 0,
		consecutive_failures INTEGER NOT NULL DEFAULT 0,
		max_retries INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS cron_run_logs (
		id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		status TEXT NOT NULL,
		error TEXT,
		output TEXT,
		delivery_mode TEXT,
		delivered_to TEXT,
		FOREIGN KEY (job_id) REFERENCES cron_jobs(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_cron_run_logs_job_id ON cron_run_logs(job_id);
	CREATE INDEX IF NOT EXISTS idx_cron_run_logs_started_at ON cron_run_logs(started_at DESC);
	`

	_, err := db.Exec(schema)
	return err
}
