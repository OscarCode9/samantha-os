package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/cron"
)

// cronServiceKey es el clave para almacenar el CronService en el contexto.
type cronServiceKey struct{}

// CronHandler registra los endpoints de cron en el mux.
func registerCronHandlers(mux *http.ServeMux, paths config.Paths) error {
	// Inicializar servicio de cron con almacenamiento SQLite.
	cronStorePath := filepath.Join(paths.WorkspaceDir, "cron.db")
	store, err := cron.NewSQLiteStorage(cronStorePath)
	if err != nil {
		return fmt.Errorf("initialize cron storage: %w", err)
	}

	service, err := cron.NewService(context.Background(), store)
	if err != nil {
		return fmt.Errorf("initialize cron service: %w", err)
	}

	// Iniciar el servicio de cron.
	if err := service.Start(); err != nil {
		return fmt.Errorf("start cron service: %w", err)
	}

	// GET /cron/status
	mux.HandleFunc("/cron/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		status := service.Status(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// GET /cron/jobs
	mux.HandleFunc("/cron/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		opts := cron.ListJobsOptions{Limit: 50}
		jobs, err := service.ListJobs(r.Context(), opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"count": len(jobs),
			"jobs":  jobs,
		})
	})

	// POST /cron/jobs
	mux.HandleFunc("/cron/jobs/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var input cron.CreateJobInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		job, err := service.CreateJob(r.Context(), input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(job)
	})

	// GET /cron/jobs/{jobID}
	mux.HandleFunc("/cron/jobs/", func(w http.ResponseWriter, r *http.Request) {
		jobID := strings.TrimPrefix(r.URL.Path, "/cron/jobs/")
		if jobID == "" || strings.Contains(jobID, "/") {
			http.Error(w, "invalid job id", http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodGet {
			job, err := service.GetJob(r.Context(), jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(job)
			return
		}

		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			var input cron.UpdateJobInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}

			job, err := service.UpdateJob(r.Context(), jobID, input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(job)
			return
		}

		if r.Method == http.MethodDelete {
			err := service.DeleteJob(r.Context(), jobID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"deleted": jobID})
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// POST /cron/jobs/{jobID}/run
	mux.HandleFunc("/cron/jobs/run/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		jobID := strings.TrimPrefix(r.URL.Path, "/cron/jobs/run/")
		if jobID == "" || strings.Contains(jobID, "/") {
			http.Error(w, "invalid job id", http.StatusBadRequest)
			return
		}

		runLog, err := service.RunJobNow(r.Context(), jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(runLog)
	})

	fmt.Println("[cron] HTTP handlers registered")
	return nil
}
