package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Service gestiona la ejecución de jobs programados.
// Usa robfig/cron para el scheduling y SQLite para persistencia.
type Service struct {
	mu        sync.RWMutex
	store     Storage
	scheduler *cron.Cron
	jobs      map[string]*CronJob
	entries   map[string]cron.EntryID // map job ID to cron entry ID
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewService crea una nueva instancia del servicio de cron.
func NewService(ctx context.Context, store Storage) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}

	baseCtx, cancel := context.WithCancel(ctx)

	s := &Service{
		store:     store,
		scheduler: cron.New(cron.WithSeconds()), // Permite precisión de segundos
		jobs:      make(map[string]*CronJob),
		entries:   make(map[string]cron.EntryID),
		ctx:       baseCtx,
		cancel:    cancel,
	}

	// Cargar jobs persitidos del almacenamiento.
	if err := s.loadPersistedJobs(); err != nil {
		return nil, fmt.Errorf("load persisted jobs: %w", err)
	}

	return s, nil
}

// Start inicia el scheduler y ejecuta los jobs programados.
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("cron service already running")
	}

	// Re-inscribir todos los jobs habilitados.
	for _, job := range s.jobs {
		if job.Enabled {
			if err := s.scheduleJob(job); err != nil {
				return fmt.Errorf("schedule job %s: %w", job.ID, err)
			}
		}
	}

	s.scheduler.Start()
	s.running = true
	fmt.Printf("[cron] Service started with %d jobs\n", len(s.jobs))
	return nil
}

// Stop detiene el scheduler y cancela jobs en ejecución.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cancel()
	s.scheduler.Stop()
	s.running = false
	fmt.Println("[cron] Service stopped")
}

// CreateJob crea un nuevo trabajo.
func (s *Service) CreateJob(ctx context.Context, input CreateJobInput) (*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		return nil, fmt.Errorf("sync jobs: %w", err)
	}

	if input.Name == "" {
		return nil, fmt.Errorf("job name required")
	}

	job := &CronJob{
		ID:                  generateJobID(),
		Name:                input.Name,
		Schedule:            input.Schedule,
		Payload:             input.Payload,
		Enabled:             true,
		DeliveryMode:        input.DeliveryMode,
		WebhookURL:          input.WebhookURL,
		WebhookSecret:       input.WebhookSecret,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		RunCount:            0,
		ConsecutiveFailures: 0,
		MaxRetries:          input.MaxRetries,
	}

	// Validar que el schedule es válido.
	if err := s.validateSchedule(job.Schedule); err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}

	// Persistir job.
	if err := s.store.SaveJob(ctx, job); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}

	s.jobs[job.ID] = job

	// Si el servicio está corriendo, programar el job inmediatamente.
	if s.running {
		if err := s.scheduleJob(job); err != nil {
			return nil, fmt.Errorf("schedule job: %w", err)
		}
	}

	fmt.Printf("[cron] Job created: %s (%s)\n", job.Name, job.ID)
	return job, nil
}

// GetJob retorna un job por su ID.
func (s *Service) GetJob(ctx context.Context, jobID string) (*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		return nil, fmt.Errorf("sync jobs: %w", err)
	}

	job, ok := s.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	return job, nil
}

// ListJobs lista los jobs con opciones de filtrado.
func (s *Service) ListJobs(ctx context.Context, opts ListJobsOptions) ([]*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		return nil, fmt.Errorf("sync jobs: %w", err)
	}

	var results []*CronJob
	for _, job := range s.jobs {
		if opts.Enabled != nil && *opts.Enabled != job.Enabled {
			continue
		}
		results = append(results, job)
	}

	// Aplicar limit/offset.
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, nil
}

// UpdateJob actualiza un job existente.
func (s *Service) UpdateJob(ctx context.Context, jobID string, input UpdateJobInput) (*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		return nil, fmt.Errorf("sync jobs: %w", err)
	}

	job, ok := s.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Guardar estado anterior para determinar si re-programar.
	wasEnabled := job.Enabled

	if input.Name != nil {
		job.Name = *input.Name
	}
	if input.Schedule != nil {
		if err := s.validateSchedule(*input.Schedule); err != nil {
			return nil, fmt.Errorf("invalid schedule: %w", err)
		}
		job.Schedule = *input.Schedule
	}
	if input.Payload != nil {
		job.Payload = *input.Payload
	}
	if input.Enabled != nil {
		job.Enabled = *input.Enabled
	}
	if input.DeliveryMode != nil {
		job.DeliveryMode = *input.DeliveryMode
	}
	if input.WebhookURL != nil {
		job.WebhookURL = *input.WebhookURL
	}
	if input.WebhookSecret != nil {
		job.WebhookSecret = *input.WebhookSecret
	}
	if input.MaxRetries != nil {
		job.MaxRetries = *input.MaxRetries
	}

	job.UpdatedAt = time.Now()

	// Persistir cambios.
	if err := s.store.SaveJob(ctx, job); err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}

	// Re-programar si el estado cambió.
	if s.running && wasEnabled != job.Enabled {
		if job.Enabled {
			if err := s.scheduleJob(job); err != nil {
				return nil, fmt.Errorf("schedule job: %w", err)
			}
		} else {
			s.unscheduleJob(job.ID)
		}
	}

	fmt.Printf("[cron] Job updated: %s (%s)\n", job.Name, job.ID)
	return job, nil
}

// DeleteJob elimina un job.
func (s *Service) DeleteJob(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		return fmt.Errorf("sync jobs: %w", err)
	}

	job, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}

	// Desagendar si está programado.
	s.unscheduleJob(jobID)

	// Eliminar del almacenamiento.
	if err := s.store.DeleteJob(ctx, jobID); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	delete(s.jobs, jobID)
	fmt.Printf("[cron] Job deleted: %s (%s)\n", job.Name, jobID)
	return nil
}

// RunJobNow ejecuta un job de inmediato (bypass del schedule).
func (s *Service) RunJobNow(ctx context.Context, jobID string) (*JobRunLog, error) {
	s.mu.Lock()
	if err := s.syncFromStoreLocked(ctx); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("sync jobs: %w", err)
	}
	job, ok := s.jobs[jobID]
	s.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Ejecutar el job y registrar resultado.
	runLog := &JobRunLog{
		ID:        generateRunLogID(),
		JobID:     jobID,
		StartedAt: time.Now(),
		Status:    "running",
	}

	output, runErr := executeJobPayload(ctx, job)
	now := time.Now()
	runLog.CompletedAt = &now
	runLog.Output = output
	if runErr != nil {
		runLog.Status = "failed"
		runLog.Error = runErr.Error()
	} else {
		runLog.Status = "success"
	}
	runLog.DeliveryMode = job.DeliveryMode

	// Actualizar el job con los resultados.
	s.mu.Lock()
	job.LastRunAt = &now
	job.LastRunStatus = runLog.Status
	job.LastError = runLog.Error
	job.RunCount++
	if runErr != nil {
		job.ConsecutiveFailures++
	} else {
		job.ConsecutiveFailures = 0
	}
	err := s.store.SaveJob(ctx, job)
	s.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("save job: %w", err)
	}

	// Guardar el log de ejecución.
	if err := s.store.SaveRunLog(ctx, runLog); err != nil {
		fmt.Printf("[cron] Failed to save run log: %v\n", err)
	}

	if runErr != nil {
		fmt.Printf("[cron] Job failed: %s (%s): %v\n", job.Name, jobID, runErr)
	} else {
		fmt.Printf("[cron] Job executed: %s (%s)\n", job.Name, jobID)
	}
	return runLog, nil
}

// Status retorna el estado actual del servicio.
func (s *Service) Status(ctx context.Context) *CronStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.syncFromStoreLocked(ctx); err != nil {
		fmt.Printf("[cron] failed to sync jobs for status: %v\n", err)
	}

	var activeCount, failedCount int
	for _, job := range s.jobs {
		if job.LastRunStatus == "running" {
			activeCount++
		}
		if job.LastRunStatus == "failed" {
			failedCount++
		}
	}

	status := &CronStatus{
		Enabled:    s.running,
		TotalJobs:  len(s.jobs),
		ActiveJobs: activeCount,
		FailedJobs: failedCount,
		LastSyncAt: time.Now(),
	}

	// Encontrar el próximo job programado.
	for _, entry := range s.scheduler.Entries() {
		if status.NextScheduledAt == nil || entry.Next.Before(*status.NextScheduledAt) {
			nextTime := entry.Next
			status.NextScheduledAt = &nextTime
		}
	}

	return status
}

// Private helpers

func (s *Service) scheduleJob(job *CronJob) error {
	if !job.Enabled {
		return nil
	}

	spec, err := s.buildCronSpec(job.Schedule)
	if err != nil {
		return fmt.Errorf("build cron spec: %w", err)
	}

	entryID, err := s.scheduler.AddFunc(spec, func() {
		if _, err := s.RunJobNow(s.ctx, job.ID); err != nil {
			fmt.Printf("[cron] Failed to run job %s: %v\n", job.ID, err)
		}
	})
	if err != nil {
		return fmt.Errorf("add to scheduler: %w", err)
	}

	s.entries[job.ID] = entryID
	return nil
}

func (s *Service) unscheduleJob(jobID string) {
	if entryID, ok := s.entries[jobID]; ok {
		s.scheduler.Remove(entryID)
		delete(s.entries, jobID)
	}
}

func (s *Service) validateSchedule(sch Schedule) error {
	switch sch.Kind {
	case "cron":
		if sch.Expr == "" {
			return fmt.Errorf("cron expression required")
		}
		_, err := cron.ParseStandard(strings.TrimSpace(sch.Expr))
		return err
	case "interval":
		if sch.IntervalMs <= 0 {
			return fmt.Errorf("interval must be > 0")
		}
	case "at":
		if sch.At == "" {
			return fmt.Errorf("timestamp required")
		}
		// TODO: Validar que es un timestamp válido.
	default:
		return fmt.Errorf("unknown schedule kind: %s", sch.Kind)
	}
	return nil
}

func (s *Service) buildCronSpec(sch Schedule) (string, error) {
	switch sch.Kind {
	case "cron":
		expr := strings.TrimSpace(sch.Expr)
		if expr == "" {
			return "", fmt.Errorf("cron expression required")
		}
		// Scheduler is configured with seconds; allow user-friendly 5-field expressions.
		if len(strings.Fields(expr)) == 5 {
			return "0 " + expr, nil
		}
		return expr, nil
	case "interval":
		return fmt.Sprintf("@every %dms", sch.IntervalMs), nil
	case "at":
		// TODO: Implementar soporte para timestamps únicos.
		return "", fmt.Errorf("'at' schedule not yet implemented")
	default:
		return "", fmt.Errorf("unknown schedule kind: %s", sch.Kind)
	}
}

func (s *Service) loadPersistedJobs() error {
	jobs, err := s.store.LoadAllJobs(s.ctx)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	return nil
}

// syncFromStoreLocked reloads jobs from persistent storage and reconciles scheduler entries.
// Caller must hold s.mu.
func (s *Service) syncFromStoreLocked(ctx context.Context) error {
	jobs, err := s.store.LoadAllJobs(ctx)
	if err != nil {
		return err
	}

	for _, entryID := range s.entries {
		s.scheduler.Remove(entryID)
	}
	s.entries = make(map[string]cron.EntryID)
	s.jobs = make(map[string]*CronJob, len(jobs))

	for _, job := range jobs {
		s.jobs[job.ID] = job
		if s.running && job.Enabled {
			if err := s.scheduleJob(job); err != nil {
				return fmt.Errorf("schedule job %s: %w", job.ID, err)
			}
		}
	}

	return nil
}

func executeJobPayload(ctx context.Context, job *CronJob) (string, error) {
	switch PayloadKind(job.Payload.Kind) {
	case PayloadSystemEvent:
		return executeSystemEventPayload(ctx, job.Payload.Data)
	default:
		return "payload accepted (no-op)", nil
	}
}

func executeSystemEventPayload(ctx context.Context, raw json.RawMessage) (string, error) {
	params := struct {
		Summary string `json:"summary"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		Message string `json:"message"`
		Urgency string `json:"urgency"`
	}{}

	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &params); err != nil {
			return "", fmt.Errorf("invalid systemEvent payload: %w", err)
		}
	}

	summary := strings.TrimSpace(params.Summary)
	if summary == "" {
		summary = strings.TrimSpace(params.Title)
	}
	if summary == "" {
		summary = "Samantha"
	}

	body := strings.TrimSpace(params.Body)
	if body == "" {
		body = strings.TrimSpace(params.Message)
	}

	urgency := strings.TrimSpace(strings.ToLower(params.Urgency))
	if urgency == "" {
		urgency = "normal"
	}
	if urgency != "low" && urgency != "normal" && urgency != "critical" {
		urgency = "normal"
	}

	path, err := exec.LookPath("notify-send")
	if err != nil {
		return "", fmt.Errorf("notify-send not available: %w", err)
	}

	args := []string{
		"--urgency=" + urgency,
		"--app-name=elementary-claw",
		summary,
	}
	if body != "" {
		args = append(args, body)
	}

	cmd := exec.CommandContext(ctx, path, args...)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return "", fmt.Errorf("notify-send failed: %w (%s)", runErr, strings.TrimSpace(string(output)))
	}

	if body == "" {
		return fmt.Sprintf("notification sent: %s", summary), nil
	}
	return fmt.Sprintf("notification sent: %s — %s", summary, body), nil
}
