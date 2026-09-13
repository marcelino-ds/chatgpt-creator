// Package store provides SQLite persistence for jobs, accounts, and logs.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Job is a batch registration run.
type Job struct {
	ID           string     `json:"id"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Target       int        `json:"target"`
	Workers      int        `json:"workers"`
	Proxy        string     `json:"proxy"`
	Domain       string     `json:"domain"`
	Status       string     `json:"status"` // running, paused, completed, stopped, failed
	SuccessCount int        `json:"success_count"`
	FailureCount int        `json:"failure_count"`
	Attempts     int        `json:"attempts"`
	ElapsedMS    int64      `json:"elapsed_ms"`
}

// Account is a single registration outcome.
type Account struct {
	ID        int64     `json:"id"`
	JobID     string    `json:"job_id"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// LogEntry is a single registration step.
type LogEntry struct {
	JobID      string    `json:"job_id"`
	WorkerID   int       `json:"worker_id"`
	Tag        string    `json:"tag"`
	Step       string    `json:"step"`
	StatusCode int       `json:"status_code"`
	Timestamp  time.Time `json:"timestamp"`
}

// Open opens (and initializes) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// modernc/sqlite works best with limited connections for writes.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	target INTEGER NOT NULL,
	workers INTEGER NOT NULL,
	proxy TEXT,
	domain TEXT,
	status TEXT NOT NULL,
	success_count INTEGER DEFAULT 0,
	failure_count INTEGER DEFAULT 0,
	attempts INTEGER DEFAULT 0,
	elapsed_ms INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id TEXT NOT NULL,
	email TEXT NOT NULL,
	password TEXT NOT NULL,
	success BOOLEAN NOT NULL,
	error TEXT,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_job ON accounts(job_id);
CREATE TABLE IF NOT EXISTS logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id TEXT NOT NULL,
	worker_id INTEGER NOT NULL,
	tag TEXT,
	step TEXT NOT NULL,
	status_code INTEGER,
	timestamp TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_logs_job ON logs(job_id);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func fmtTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// CreateJob inserts a new running job.
func (s *Store) CreateJob(j *Job) error {
	j.StartedAt = time.Now()
	j.Status = "running"
	_, err := s.db.Exec(
		`INSERT INTO jobs (id, started_at, target, workers, proxy, domain, status, success_count, failure_count, attempts, elapsed_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0)`,
		j.ID, fmtTime(j.StartedAt), j.Target, j.Workers, j.Proxy, j.Domain, j.Status,
	)
	return err
}

// UpdateJobStatus updates the job's status, finish time, and counters.
func (s *Store) UpdateJobStatus(id, status string, successCount, failureCount, attempts int, elapsedMS int64) error {
	now := time.Now()
	switch status {
	case "completed", "stopped", "failed":
		_, err := s.db.Exec(
			`UPDATE jobs SET status=?, finished_at=?, success_count=?, failure_count=?, attempts=?, elapsed_ms=? WHERE id=?`,
			status, fmtTime(now), successCount, failureCount, attempts, elapsedMS, id,
		)
		return err
	default: // running, paused
		_, err := s.db.Exec(
			`UPDATE jobs SET status=?, success_count=?, failure_count=?, attempts=?, elapsed_ms=? WHERE id=?`,
			status, successCount, failureCount, attempts, elapsedMS, id,
		)
		return err
	}
}

// GetJob returns a single job by id.
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(
		`SELECT id, started_at, finished_at, target, workers, proxy, domain, status, success_count, failure_count, attempts, elapsed_ms
		 FROM jobs WHERE id=?`, id,
	)
	return scanJob(row)
}

// ListJobs returns recent jobs ordered by start time desc.
func (s *Store) ListJobs(limit int) ([]*Job, error) {
	rows, err := s.db.Query(
		`SELECT id, started_at, finished_at, target, workers, proxy, domain, status, success_count, failure_count, attempts, elapsed_ms
		 FROM jobs ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(r rowScanner) (*Job, error) {
	j := &Job{}
	var startedAt, finishedAt sql.NullString
	var proxy, domain sql.NullString
	if err := r.Scan(&j.ID, &startedAt, &finishedAt, &j.Target, &j.Workers, &proxy, &domain, &j.Status, &j.SuccessCount, &j.FailureCount, &j.Attempts, &j.ElapsedMS); err != nil {
		return nil, err
	}
	j.StartedAt, _ = parseTime(startedAt.String)
	if finishedAt.Valid {
		if t, err := parseTime(finishedAt.String); err == nil {
			j.FinishedAt = &t
		}
	}
	j.Proxy = proxy.String
	j.Domain = domain.String
	return j, nil
}

// AddAccount records a registration outcome.
func (s *Store) AddAccount(a *Account) error {
	a.CreatedAt = time.Now()
	res, err := s.db.Exec(
		`INSERT INTO accounts (job_id, email, password, success, error, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		a.JobID, a.Email, a.Password, a.Success, a.Error, fmtTime(a.CreatedAt),
	)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// ListAccounts returns accounts for a job, most recent first.
func (s *Store) ListAccounts(jobID string, limit int) ([]*Account, error) {
	rows, err := s.db.Query(
		`SELECT id, job_id, email, password, success, COALESCE(error,''), created_at
		 FROM accounts WHERE job_id=? ORDER BY id DESC LIMIT ?`, jobID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Account
	for rows.Next() {
		a := &Account{}
		var created string
		if err := rows.Scan(&a.ID, &a.JobID, &a.Email, &a.Password, &a.Success, &a.Error, &created); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// AllAccounts returns accounts across all jobs, most recent first.
func (s *Store) AllAccounts(limit int) ([]*Account, error) {
	rows, err := s.db.Query(
		`SELECT id, job_id, email, password, success, COALESCE(error,''), created_at
		 FROM accounts ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Account
	for rows.Next() {
		a := &Account{}
		var created string
		if err := rows.Scan(&a.ID, &a.JobID, &a.Email, &a.Password, &a.Success, &a.Error, &created); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddLog records a single step log.
func (s *Store) AddLog(jobID string, workerID int, tag, step string, statusCode int) error {
	_, err := s.db.Exec(
		`INSERT INTO logs (job_id, worker_id, tag, step, status_code, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
		jobID, workerID, tag, step, statusCode, fmtTime(time.Now()),
	)
	return err
}

// ListLogs returns logs for a job, oldest first.
func (s *Store) ListLogs(jobID string, limit int) ([]*LogEntry, error) {
	rows, err := s.db.Query(
		`SELECT job_id, worker_id, COALESCE(tag,''), step, COALESCE(status_code,0), timestamp
		 FROM logs WHERE job_id=? ORDER BY id ASC LIMIT ?`, jobID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*LogEntry
	for rows.Next() {
		l := &LogEntry{}
		var ts string
		if err := rows.Scan(&l.JobID, &l.WorkerID, &l.Tag, &l.Step, &l.StatusCode, &ts); err != nil {
			return nil, err
		}
		l.Timestamp, _ = parseTime(ts)
		out = append(out, l)
	}
	return out, rows.Err()
}
