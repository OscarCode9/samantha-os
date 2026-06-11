package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	edsTaskListDefaultUID = "system-task-list"
)

func openSystemTaskListClient(ctx context.Context, run commandRunner) (objectPath string, busName string, err error) {
	if run == nil {
		run = defaultCommandRunner
	}

	candidates := []string{
		strings.TrimSpace(edsTaskListDefaultUID),
		"system-tasks",
		"tasks",
		"local",
	}

	for _, uid := range candidates {
		if uid == "" {
			continue
		}
		output, callErr := run(ctx,
			"busctl", "--user", "--json=short", "call",
			edsCalendarBus,
			edsCalendarFactory,
			"org.gnome.evolution.dataserver.CalendarFactory",
			"OpenTaskList",
			"s",
			uid,
		)
		if callErr != nil {
			continue
		}

		var response struct {
			Type string   `json:"type"`
			Data []string `json:"data"`
		}
		if err := json.Unmarshal(output, &response); err != nil {
			continue
		}
		if len(response.Data) >= 2 {
			return response.Data[0], response.Data[1], nil
		}
	}

	return "", "", fmt.Errorf("unable to open task list source; tried %s", strings.Join(candidates, ", "))
}

func listTaskItems(ctx context.Context, run commandRunner, query string) ([]string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if strings.TrimSpace(query) == "" {
		query = "#t"
	}

	objectPath, busName, err := openSystemTaskListClient(ctx, run)
	if err != nil {
		return nil, err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsCalendarIface,
		"GetObjectList",
		"s",
		query,
	)
	if callErr != nil {
		return nil, fmt.Errorf("list task items failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string     `json:"type"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("parse GetObjectList response: %w", err)
	}
	if len(response.Data) == 0 {
		return []string{}, nil
	}
	return response.Data[0], nil
}

func createTaskItem(ctx context.Context, run commandRunner, ics string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if strings.TrimSpace(ics) == "" {
		return "", fmt.Errorf("ics payload is required")
	}

	objectPath, busName, err := openSystemTaskListClient(ctx, run)
	if err != nil {
		return "", err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsCalendarIface,
		"CreateObjects",
		"asu",
		"1",
		ics,
		"0",
	)
	if callErr != nil {
		return "", fmt.Errorf("create task item failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string          `json:"type"`
		Data [][]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err == nil {
		if len(response.Data) > 0 && len(response.Data[0]) > 0 {
			if uid, ok := response.Data[0][0].(string); ok && strings.TrimSpace(uid) != "" {
				return uid, nil
			}
		}
	}

	uid := parseICSField(ics, "UID")
	if uid == "" {
		return "", nil
	}
	return uid, nil
}

func getTaskItem(ctx context.Context, run commandRunner, uid string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return "", fmt.Errorf("uid is required")
	}

	objectPath, busName, err := openSystemTaskListClient(ctx, run)
	if err != nil {
		return "", err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsCalendarIface,
		"GetObject",
		"ss",
		uid,
		"",
	)
	if callErr != nil {
		return "", fmt.Errorf("get task item failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string   `json:"type"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("parse GetObject response: %w", err)
	}
	if len(response.Data) == 0 {
		return "", fmt.Errorf("task not found")
	}
	return response.Data[0], nil
}

func modifyTaskItem(ctx context.Context, run commandRunner, ics string) error {
	if run == nil {
		run = defaultCommandRunner
	}
	if strings.TrimSpace(ics) == "" {
		return fmt.Errorf("ics payload is required")
	}

	objectPath, busName, err := openSystemTaskListClient(ctx, run)
	if err != nil {
		return err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsCalendarIface,
		"ModifyObjects",
		"assu",
		"1",
		ics,
		"this",
		"0",
	)
	if callErr != nil {
		return fmt.Errorf("modify task item failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteTaskItem(ctx context.Context, run commandRunner, uid string) error {
	if run == nil {
		run = defaultCommandRunner
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return fmt.Errorf("uid is required")
	}

	objectPath, busName, err := openSystemTaskListClient(ctx, run)
	if err != nil {
		return err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsCalendarIface,
		"RemoveObjects",
		"a(ss)su",
		"1",
		uid,
		"",
		"this",
		"0",
	)
	if callErr != nil {
		return fmt.Errorf("delete task item failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func buildTaskICS(uid, title, description, dueRFC3339, timezone string) (string, error) {
	uid = strings.TrimSpace(uid)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	timezone = strings.TrimSpace(timezone)
	if uid == "" {
		uid = generateCalendarUID()
	}
	if title == "" {
		return "", fmt.Errorf("title is required")
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VTODO",
		"UID:" + uid,
		"DTSTAMP:" + stamp,
		"SUMMARY:" + escapeICS(title),
		"STATUS:NEEDS-ACTION",
	}
	if description != "" {
		lines = append(lines, "DESCRIPTION:"+escapeICS(description))
	}

	if strings.TrimSpace(dueRFC3339) != "" {
		due, err := time.Parse(time.RFC3339, strings.TrimSpace(dueRFC3339))
		if err != nil {
			return "", fmt.Errorf("due must be RFC3339, for example 2026-04-29T17:00:00-06:00")
		}
		if timezone != "" {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				return "", fmt.Errorf("timezone must be a valid IANA zone, for example America/Mexico_City")
			}
			due = due.In(loc)
			lines = append(lines, "DUE;TZID="+loc.String()+":"+due.Format("20060102T150405"))
		} else {
			lines = append(lines, "DUE:"+due.UTC().Format("20060102T150405Z"))
		}
	}

	lines = append(lines, "END:VTODO")
	return strings.Join(lines, "\r\n"), nil
}

func setTaskCompleted(ics string, completed bool) string {
	trimmed := strings.TrimSpace(ics)
	if trimmed == "" {
		return ics
	}

	lines := strings.Split(ics, "\r\n")
	statusSet := false
	completedSet := false
	result := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		if strings.HasPrefix(line, "STATUS:") {
			if completed {
				result = append(result, "STATUS:COMPLETED")
			} else {
				result = append(result, "STATUS:NEEDS-ACTION")
			}
			statusSet = true
			continue
		}
		if strings.HasPrefix(line, "COMPLETED:") {
			if completed {
				result = append(result, "COMPLETED:"+time.Now().UTC().Format("20060102T150405Z"))
				completedSet = true
			}
			continue
		}
		result = append(result, line)
	}

	if !statusSet {
		if completed {
			result = append(result, "STATUS:COMPLETED")
		} else {
			result = append(result, "STATUS:NEEDS-ACTION")
		}
	}
	if completed && !completedSet {
		result = append(result, "COMPLETED:"+time.Now().UTC().Format("20060102T150405Z"))
	}
	return strings.Join(result, "\r\n")
}
