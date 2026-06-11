package cron

import (
	"encoding/json"
	"time"
)

// Schedule describe la frecuencia de ejecución de un job.
// Soporta three tipos:
// - "cron": expresión cron (ej: "0 9 * * *")
// - "interval": cada N miliseconds
// - "at": timestamp ISO8601 o Unix epoch
type Schedule struct {
	Kind       string `json:"kind"` // "cron", "interval", "at"
	Expr       string `json:"expr,omitempty"`
	IntervalMs int64  `json:"intervalMs,omitempty"`
	At         string `json:"at,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	StaggerMs  int64  `json:"staggerMs,omitempty"`
}

// PayloadKind define el tipo de carga útil que el job ejecutará.
type PayloadKind string

const (
	PayloadSystemEvent PayloadKind = "systemEvent"
	PayloadWebhook     PayloadKind = "webhook"
	PayloadAgentTurn   PayloadKind = "agentTurn"
)

// Payload contiene los datos para ejecutar un job.
type Payload struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// DeliveryMode especifica cómo se notifican los resultados.
type DeliveryMode string

const (
	DeliveryNone     DeliveryMode = "none"     // No notificar
	DeliveryAnnounce DeliveryMode = "announce" // Notificación en memoria
	DeliveryWebhook  DeliveryMode = "webhook"  // POST webhook
)

// CronJob describe un trabajo que se ejecuta según su schedule.
type CronJob struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Schedule            Schedule     `json:"schedule"`
	Payload             Payload      `json:"payload"`
	Enabled             bool         `json:"enabled"`
	DeliveryMode        DeliveryMode `json:"deliveryMode"`
	WebhookURL          string       `json:"webhookUrl,omitempty"`
	WebhookSecret       string       `json:"webhookSecret,omitempty"`
	CreatedAt           time.Time    `json:"createdAt"`
	UpdatedAt           time.Time    `json:"updatedAt"`
	LastRunAt           *time.Time   `json:"lastRunAt,omitempty"`
	LastRunStatus       string       `json:"lastRunStatus,omitempty"` // "success", "failed", "running"
	LastError           string       `json:"lastError,omitempty"`
	NextRunAt           *time.Time   `json:"nextRunAt,omitempty"`
	RunCount            int64        `json:"runCount"`
	ConsecutiveFailures int          `json:"consecutiveFailures"`
	MaxRetries          int          `json:"maxRetries"`
}

// CronStatus agrupa el estado global del servicio de cron.
type CronStatus struct {
	Enabled         bool       `json:"enabled"`
	TotalJobs       int        `json:"totalJobs"`
	ActiveJobs      int        `json:"activeJobs"`
	FailedJobs      int        `json:"failedJobs"`
	LastSyncAt      time.Time  `json:"lastSyncAt"`
	NextScheduledAt *time.Time `json:"nextScheduledAt,omitempty"`
}

// JobRunLog registra cada ejecución de un job.
type JobRunLog struct {
	ID           string       `json:"id"`
	JobID        string       `json:"jobId"`
	StartedAt    time.Time    `json:"startedAt"`
	CompletedAt  *time.Time   `json:"completedAt,omitempty"`
	Status       string       `json:"status"` // "running", "success", "failed"
	Error        string       `json:"error,omitempty"`
	Output       string       `json:"output,omitempty"`
	DeliveryMode DeliveryMode `json:"deliveryMode"`
	DeliveredTo  string       `json:"deliveredTo,omitempty"`
}

// CreateJobInput es el payload para crear un nuevo job.
type CreateJobInput struct {
	Name          string       `json:"name"`
	Schedule      Schedule     `json:"schedule"`
	Payload       Payload      `json:"payload"`
	DeliveryMode  DeliveryMode `json:"deliveryMode"`
	WebhookURL    string       `json:"webhookUrl,omitempty"`
	WebhookSecret string       `json:"webhookSecret,omitempty"`
	MaxRetries    int          `json:"maxRetries"`
}

// UpdateJobInput es el payload para actualizar un job.
type UpdateJobInput struct {
	Name          *string       `json:"name,omitempty"`
	Schedule      *Schedule     `json:"schedule,omitempty"`
	Payload       *Payload      `json:"payload,omitempty"`
	Enabled       *bool         `json:"enabled,omitempty"`
	DeliveryMode  *DeliveryMode `json:"deliveryMode,omitempty"`
	WebhookURL    *string       `json:"webhookUrl,omitempty"`
	WebhookSecret *string       `json:"webhookSecret,omitempty"`
	MaxRetries    *int          `json:"maxRetries,omitempty"`
}

// ListJobsOptions filtra la lista de jobs.
type ListJobsOptions struct {
	Enabled *bool
	Limit   int
	Offset  int
}
