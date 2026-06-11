package cron

import (
	"context"
)

// Storage define la interfaz para persistir datos de jobs.
type Storage interface {
	// Job operations
	SaveJob(ctx context.Context, job *CronJob) error
	LoadAllJobs(ctx context.Context) ([]*CronJob, error)
	GetJob(ctx context.Context, jobID string) (*CronJob, error)
	DeleteJob(ctx context.Context, jobID string) error

	// Run log operations
	SaveRunLog(ctx context.Context, log *JobRunLog) error
	ListRunLogs(ctx context.Context, jobID string, limit int) ([]*JobRunLog, error)

	// Housekeeping
	Close() error
}
