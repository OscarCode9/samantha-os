package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestParseCalendarEventICS(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VEVENT",
		"UID:sam-demo-1",
		"DTSTART;TZID=America/Mexico_City:20260430T170000",
		"DTEND;TZID=America/Mexico_City:20260430T180000",
		"SUMMARY:Demo\\, review",
		"DESCRIPTION:Bring\\nnotes",
		"LOCATION:Room\\;A",
		"RRULE:FREQ=DAILY",
		"END:VEVENT",
	}, "\r\n")

	event, err := parseCalendarEventICS(ics)
	if err != nil {
		t.Fatal(err)
	}
	if event.UID != "sam-demo-1" {
		t.Fatalf("unexpected uid: %q", event.UID)
	}
	if event.Title != "Demo, review" {
		t.Fatalf("unexpected title: %q", event.Title)
	}
	if event.Description != "Bring\nnotes" {
		t.Fatalf("unexpected description: %q", event.Description)
	}
	if event.Location != "Room;A" {
		t.Fatalf("unexpected location: %q", event.Location)
	}
	if event.Timezone != "America/Mexico_City" {
		t.Fatalf("unexpected timezone: %q", event.Timezone)
	}
	if !event.Daily {
		t.Fatal("expected daily recurrence")
	}
	if event.AllDay {
		t.Fatal("expected timed event")
	}
}

func TestModifyCalendarEventToolUpdatesExistingEvent(t *testing.T) {
	existingICS := strings.Join([]string{
		"BEGIN:VEVENT",
		"UID:sam-demo-1",
		"DTSTART:20260430T170000Z",
		"DTEND:20260430T180000Z",
		"SUMMARY:Old Demo",
		"DESCRIPTION:Old note",
		"LOCATION:Desk",
		"END:VEVENT",
	}, "\r\n")

	var modifiedICS string
	tool := &modifyCalendarEventTool{
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "busctl" {
				t.Fatalf("unexpected command: %s", name)
			}
			if len(args) < 7 {
				t.Fatalf("unexpected args: %#v", args)
			}
			switch args[6] {
			case "OpenCalendar":
				return []byte(`{"type":"(ss)","data":["/mock/calendar","org.mock.Calendar"]}`), nil
			case "GetObject":
				if got := args[8]; got != "sam-demo-1" {
					t.Fatalf("unexpected uid lookup: %q", got)
				}
				return []byte(fmt.Sprintf(`{"type":"(s)","data":[%q]}`, existingICS)), nil
			case "ModifyObjects":
				modifiedICS = args[9]
				return []byte(`{"type":"()","data":[]}`), nil
			default:
				t.Fatalf("unexpected busctl method: %s", args[6])
				return nil, nil
			}
		},
	}

	result := tool.Execute(context.Background(), `{"uid":"sam-demo-1","title":"Updated Demo","description":"","start":"2026-04-30T18:30:00Z","duration_minutes":90,"location":"Board room","daily":true}`)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if modifiedICS == "" {
		t.Fatal("expected ModifyObjects payload")
	}
	if !strings.Contains(modifiedICS, "UID:sam-demo-1") {
		t.Fatalf("missing uid in modified ICS: %s", modifiedICS)
	}
	if !strings.Contains(modifiedICS, "SUMMARY:Updated Demo") {
		t.Fatalf("missing updated summary in modified ICS: %s", modifiedICS)
	}
	if strings.Contains(modifiedICS, "DESCRIPTION:") {
		t.Fatalf("expected description to be cleared, got: %s", modifiedICS)
	}
	if !strings.Contains(modifiedICS, "LOCATION:Board room") {
		t.Fatalf("missing updated location in modified ICS: %s", modifiedICS)
	}
	if !strings.Contains(modifiedICS, "DTSTART:20260430T183000Z") {
		t.Fatalf("missing updated start in modified ICS: %s", modifiedICS)
	}
	if !strings.Contains(modifiedICS, "DTEND:20260430T200000Z") {
		t.Fatalf("missing updated end in modified ICS: %s", modifiedICS)
	}
	if !strings.Contains(modifiedICS, "RRULE:FREQ=DAILY") {
		t.Fatalf("missing daily recurrence in modified ICS: %s", modifiedICS)
	}
	if !strings.Contains(result.Content, `"calendar_uid": "sam-demo-1"`) {
		t.Fatalf("unexpected tool result: %s", result.Content)
	}
}