package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type deleteAllJobsAndEventsTool struct {
	baseURL string
	client  *http.Client
	run     commandRunner
}

func NewDeleteAllJobsAndEventsTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &deleteAllJobsAndEventsTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 20 * time.Second},
		run:     defaultCommandRunner,
	}
}

func (t *deleteAllJobsAndEventsTool) Name() string { return "delete_all_jobs_and_events" }

func (t *deleteAllJobsAndEventsTool) Description() string {
	return "Delete all cron jobs and their linked calendar events created by Samantha."
}

func (t *deleteAllJobsAndEventsTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"delete_all_calendar_sam_events": {
				Type:        "boolean",
				Description: "When true, also delete all calendar events whose UID starts with 'sam-'.",
			},
			"delete_all_calendar_events": {
				Type:        "boolean",
				Description: "When true, delete ALL events in the current system calendar (destructive).",
			},
		},
	}
}

func (t *deleteAllJobsAndEventsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		DeleteAllCalendarSamEvents bool `json:"delete_all_calendar_sam_events"`
		DeleteAllCalendarEvents    bool `json:"delete_all_calendar_events"`
	}{}
	if strings.TrimSpace(arguments) != "" {
		if err := ParseArgs(arguments, &params); err != nil {
			return ErrorResult(err.Error())
		}
	}

	jobs, err := t.fetchJobs(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}

	jobIDs := make([]string, 0, len(jobs))
	calendarUIDSet := map[string]struct{}{}
	for _, job := range jobs {
		if id, _ := job["id"].(string); strings.TrimSpace(id) != "" {
			jobIDs = append(jobIDs, id)
		}
		if uid := extractCalendarUIDFromJob(job); uid != "" {
			calendarUIDSet[uid] = struct{}{}
		}
	}

	deletedJobs := make([]string, 0, len(jobIDs))
	jobDeleteErrors := make([]string, 0)
	for _, id := range jobIDs {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodDelete, t.baseURL+"/cron/jobs/"+id, nil)
		if reqErr != nil {
			jobDeleteErrors = append(jobDeleteErrors, fmt.Sprintf("%s: %v", id, reqErr))
			continue
		}
		resp, doErr := t.client.Do(req)
		if doErr != nil {
			jobDeleteErrors = append(jobDeleteErrors, fmt.Sprintf("%s: %v", id, doErr))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			jobDeleteErrors = append(jobDeleteErrors, fmt.Sprintf("%s: HTTP %d", id, resp.StatusCode))
			continue
		}
		deletedJobs = append(deletedJobs, id)
	}

	if params.DeleteAllCalendarSamEvents || params.DeleteAllCalendarEvents {
		icsList, listErr := listCalendarEvents(ctx, t.run, "#t")
		if listErr == nil {
			for _, ics := range icsList {
				uid := parseICSField(ics, "UID")
				summary := strings.ToLower(parseICSField(ics, "SUMMARY"))
				if params.DeleteAllCalendarEvents || strings.HasPrefix(uid, "sam-") || strings.Contains(summary, "samantha") {
					calendarUIDSet[uid] = struct{}{}
				}
			}
		}
	}

	deletedCalendar := make([]string, 0, len(calendarUIDSet))
	calendarDeleteErrors := make([]string, 0)
	for uid := range calendarUIDSet {
		if strings.TrimSpace(uid) == "" {
			continue
		}
		if err := deleteCalendarEvent(ctx, t.run, uid); err != nil {
			calendarDeleteErrors = append(calendarDeleteErrors, fmt.Sprintf("%s: %v", uid, err))
			continue
		}
		deletedCalendar = append(deletedCalendar, uid)
	}

	return JSONResult(map[string]any{
		"deletedJobs":           deletedJobs,
		"jobDeleteErrors":       jobDeleteErrors,
		"deletedCalendarEvents": deletedCalendar,
		"calendarDeleteErrors":  calendarDeleteErrors,
	})
}

func (t *deleteAllJobsAndEventsTool) fetchJobs(ctx context.Context) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/cron/jobs", nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cron API returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Jobs []map[string]any `json:"jobs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if parsed.Jobs == nil {
		return []map[string]any{}, nil
	}
	return parsed.Jobs, nil
}
