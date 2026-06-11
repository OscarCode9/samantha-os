package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"time"
)

const (
	edsCalendarBus     = "org.gnome.evolution.dataserver.Calendar8"
	edsCalendarFactory = "/org/gnome/evolution/dataserver/CalendarFactory"
	edsCalendarIface   = "org.gnome.evolution.dataserver.Calendar"
)

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type calendarEvent struct {
	UID         string
	Title       string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
	Timezone    string
	Daily       bool
	AllDay      bool
}

func generateCalendarUID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 24)
	for i := range b {
		n, err := rand.Int(rand.Reader, bigInt(int64(len(letters))))
		if err != nil {
			b[i] = letters[i%len(letters)]
			continue
		}
		b[i] = letters[n.Int64()]
	}
	return "sam-" + string(b)
}

func bigInt(v int64) *big.Int {
	return big.NewInt(v)
}

func openSystemCalendarClient(ctx context.Context, run commandRunner) (objectPath string, busName string, err error) {
	if run == nil {
		run = defaultCommandRunner
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		edsCalendarBus,
		edsCalendarFactory,
		"org.gnome.evolution.dataserver.CalendarFactory",
		"OpenCalendar",
		"s",
		"system-calendar",
	)
	if callErr != nil {
		return "", "", fmt.Errorf("open calendar client failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string   `json:"type"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", "", fmt.Errorf("parse OpenCalendar response: %w", err)
	}
	if len(response.Data) < 2 {
		return "", "", fmt.Errorf("unexpected OpenCalendar response")
	}

	return response.Data[0], response.Data[1], nil
}

func createCalendarEvent(ctx context.Context, run commandRunner, event calendarEvent) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}

	if strings.TrimSpace(event.UID) == "" {
		event.UID = generateCalendarUID()
	}
	if strings.TrimSpace(event.Title) == "" {
		return "", fmt.Errorf("event title is required")
	}
	if event.End.Before(event.Start) || event.End.Equal(event.Start) {
		return "", fmt.Errorf("event end must be after start")
	}

	objectPath, busName, err := openSystemCalendarClient(ctx, run)
	if err != nil {
		return "", err
	}

	ics := buildEventICS(event)
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
		return "", fmt.Errorf("create calendar event failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string          `json:"type"`
		Data [][]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		// Fall back to the requested UID even when parser cannot decode output shape.
		return event.UID, nil
	}

	if len(response.Data) > 0 && len(response.Data[0]) > 0 {
		if uid, ok := response.Data[0][0].(string); ok && strings.TrimSpace(uid) != "" {
			return uid, nil
		}
	}

	return event.UID, nil
}

func listCalendarEvents(ctx context.Context, run commandRunner, query string) ([]string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if strings.TrimSpace(query) == "" {
		query = "#t"
	}

	objectPath, busName, err := openSystemCalendarClient(ctx, run)
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
		return nil, fmt.Errorf("list calendar events failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
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

func getCalendarEvent(ctx context.Context, run commandRunner, uid string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return "", fmt.Errorf("calendar uid is required")
	}

	objectPath, busName, err := openSystemCalendarClient(ctx, run)
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
		return "", fmt.Errorf("get calendar event failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string   `json:"type"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("parse GetObject response: %w", err)
	}
	if len(response.Data) == 0 || strings.TrimSpace(response.Data[0]) == "" {
		return "", fmt.Errorf("calendar event not found")
	}
	return response.Data[0], nil
}

func modifyCalendarEvent(ctx context.Context, run commandRunner, ics string) error {
	if run == nil {
		run = defaultCommandRunner
	}
	if strings.TrimSpace(ics) == "" {
		return fmt.Errorf("ics payload is required")
	}

	objectPath, busName, err := openSystemCalendarClient(ctx, run)
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
		return fmt.Errorf("modify calendar event failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteCalendarEvent(ctx context.Context, run commandRunner, uid string) error {
	if run == nil {
		run = defaultCommandRunner
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return fmt.Errorf("calendar uid is required")
	}

	objectPath, busName, err := openSystemCalendarClient(ctx, run)
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
		return fmt.Errorf("delete calendar event failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return nil
}

func buildEventICS(event calendarEvent) string {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VEVENT",
		"UID:" + event.UID,
		"DTSTAMP:" + stamp,
	}

	if event.AllDay {
		startDate := event.Start.Format("20060102")
		endDate := event.End.Format("20060102")
		lines = append(lines,
			"DTSTART;VALUE=DATE:"+startDate,
			"DTEND;VALUE=DATE:"+endDate,
		)
	} else if strings.TrimSpace(event.Timezone) != "" {
		loc, err := time.LoadLocation(strings.TrimSpace(event.Timezone))
		if err == nil {
			event.Start = event.Start.In(loc)
			event.End = event.End.In(loc)
			event.Timezone = loc.String()
		}
		lines = append(lines,
			"DTSTART;TZID="+event.Timezone+":"+event.Start.Format("20060102T150405"),
			"DTEND;TZID="+event.Timezone+":"+event.End.Format("20060102T150405"),
		)
	} else {
		startUTC := event.Start.UTC()
		endUTC := event.End.UTC()
		lines = append(lines,
			"DTSTART:"+startUTC.Format("20060102T150405Z"),
			"DTEND:"+endUTC.Format("20060102T150405Z"),
		)
	}

	lines = append(lines, "SUMMARY:"+escapeICS(event.Title))
	if strings.TrimSpace(event.Description) != "" {
		lines = append(lines, "DESCRIPTION:"+escapeICS(event.Description))
	}
	if strings.TrimSpace(event.Location) != "" {
		lines = append(lines, "LOCATION:"+escapeICS(event.Location))
	}
	if event.Daily {
		lines = append(lines, "RRULE:FREQ=DAILY")
	}

	lines = append(lines, "END:VEVENT")
	return strings.Join(lines, "\r\n")
}

func escapeICS(value string) string {
	trimmed := strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\n", "\\n",
		"\r", "",
	)
	return replacer.Replace(trimmed)
}

func unescapeICS(value string) string {
	replacer := strings.NewReplacer(
		"\\n", "\n",
		"\\,", ",",
		"\\;", ";",
		"\\\\", "\\",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func splitICSLines(ics string) []string {
	normalized := strings.ReplaceAll(ics, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func findICSLine(ics string, key string) string {
	for _, line := range splitICSLines(ics) {
		if !strings.HasPrefix(line, key) {
			continue
		}
		if len(line) <= len(key) {
			continue
		}
		next := line[len(key)]
		if next != ':' && next != ';' {
			continue
		}
		return strings.TrimSpace(line)
	}
	return ""
}

func parseICSField(ics string, key string) string {
	line := findICSLine(ics, key)
	if line != "" {
		idx := strings.IndexByte(line, ':')
		if idx >= 0 && idx+1 < len(line) {
			return strings.TrimSpace(line[idx+1:])
		}
	}
	return ""
}

func parseCalendarEventICS(ics string) (calendarEvent, error) {
	event := calendarEvent{
		UID:         strings.TrimSpace(parseICSField(ics, "UID")),
		Title:       unescapeICS(parseICSField(ics, "SUMMARY")),
		Description: unescapeICS(parseICSField(ics, "DESCRIPTION")),
		Location:    unescapeICS(parseICSField(ics, "LOCATION")),
		Daily:       strings.Contains(strings.ToUpper(ics), "RRULE:FREQ=DAILY"),
	}

	start, timezone, allDay, err := parseCalendarICSTime(findICSLine(ics, "DTSTART"))
	if err != nil {
		return calendarEvent{}, err
	}
	end, endTimezone, endAllDay, err := parseCalendarICSTime(findICSLine(ics, "DTEND"))
	if err != nil {
		return calendarEvent{}, err
	}
	if allDay != endAllDay {
		return calendarEvent{}, fmt.Errorf("calendar event DTSTART/DTEND all-day flags do not match")
	}
	if timezone == "" {
		timezone = endTimezone
	}

	event.Start = start
	event.End = end
	event.Timezone = timezone
	event.AllDay = allDay
	return event, nil
}

func parseCalendarICSTime(line string) (time.Time, string, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return time.Time{}, "", false, fmt.Errorf("calendar event is missing time field")
	}
	idx := strings.IndexByte(line, ':')
	if idx < 0 || idx+1 >= len(line) {
		return time.Time{}, "", false, fmt.Errorf("invalid calendar time field %q", line)
	}

	header := line[:idx]
	value := strings.TrimSpace(line[idx+1:])
	parts := strings.Split(header, ";")
	timezone := ""
	allDay := false
	for _, part := range parts[1:] {
		upper := strings.ToUpper(strings.TrimSpace(part))
		switch {
		case upper == "VALUE=DATE":
			allDay = true
		case strings.HasPrefix(upper, "TZID="):
			timezone = strings.TrimSpace(part[len("TZID="):])
		}
	}

	if allDay {
		parsed, err := time.Parse("20060102", value)
		if err != nil {
			return time.Time{}, timezone, true, fmt.Errorf("parse all-day calendar date: %w", err)
		}
		return parsed.UTC(), timezone, true, nil
	}

	if strings.HasSuffix(value, "Z") {
		parsed, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, timezone, false, fmt.Errorf("parse utc calendar time: %w", err)
		}
		return parsed.UTC(), timezone, false, nil
	}

	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			parsed, parseErr := time.ParseInLocation("20060102T150405", value, loc)
			if parseErr != nil {
				return time.Time{}, timezone, false, fmt.Errorf("parse zoned calendar time: %w", parseErr)
			}
			return parsed, timezone, false, nil
		}
	}

	parsed, err := time.ParseInLocation("20060102T150405", value, time.UTC)
	if err != nil {
		return time.Time{}, timezone, false, fmt.Errorf("parse floating calendar time: %w", err)
	}
	return parsed.UTC(), timezone, false, nil
}
