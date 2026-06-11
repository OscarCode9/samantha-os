package tools

import (
	"strings"
	"testing"
)

func TestBuildContactSearchQuery(t *testing.T) {
	query := buildContactSearchQuery(`alice"smith@example.com`)
	if !strings.Contains(query, `contains "full_name" "alice\"smith@example.com"`) {
		t.Fatalf("unexpected query: %s", query)
	}
	if !strings.Contains(query, `contains "email" "alice\"smith@example.com"`) {
		t.Fatalf("unexpected query: %s", query)
	}
}

func TestBuildContactVCardRoundTrip(t *testing.T) {
	record := contactRecord{
		UID:          "contact-123",
		FullName:     "Ada Lovelace",
		GivenName:    "Ada",
		FamilyName:   "Lovelace",
		Nickname:     "Countess",
		Emails:       []string{"ada@example.com", "ada@work.example"},
		Phones:       []string{"+52 55 1111 2222"},
		Organization: "Analytical Engines",
		Title:        "Mathematician",
		Note:         "First programmer",
	}

	vcard, err := buildContactVCard(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed := parseContactVCard(vcard)
	if parsed.UID != record.UID {
		t.Fatalf("unexpected uid: %q", parsed.UID)
	}
	if parsed.FullName != record.FullName {
		t.Fatalf("unexpected full name: %q", parsed.FullName)
	}
	if parsed.GivenName != record.GivenName || parsed.FamilyName != record.FamilyName {
		t.Fatalf("unexpected name fields: %#v", parsed)
	}
	if len(parsed.Emails) != 2 || parsed.Emails[0] != "ada@example.com" {
		t.Fatalf("unexpected emails: %#v", parsed.Emails)
	}
	if len(parsed.Phones) != 1 || parsed.Phones[0] != "+52 55 1111 2222" {
		t.Fatalf("unexpected phones: %#v", parsed.Phones)
	}
	if parsed.Organization != record.Organization || parsed.Title != record.Title || parsed.Note != record.Note {
		t.Fatalf("unexpected parsed contact: %#v", parsed)
	}
}

func TestApplyContactMutation(t *testing.T) {
	base := contactRecord{
		UID:        "contact-123",
		FullName:   "Ada Lovelace",
		GivenName:  "Ada",
		FamilyName: "Lovelace",
		Emails:     []string{"ada@example.com"},
		Phones:     []string{"111"},
		Title:      "Mathematician",
	}
	updated := applyContactMutation(base, contactMutation{
		FullName: "Ada Byron",
		Emails:   []string{"ada@new.example"},
		Phone:    "222",
		Title:    "Visionary",
	})
	if updated.FullName != "Ada Byron" {
		t.Fatalf("unexpected full name: %q", updated.FullName)
	}
	if len(updated.Emails) != 1 || updated.Emails[0] != "ada@new.example" {
		t.Fatalf("unexpected emails: %#v", updated.Emails)
	}
	if len(updated.Phones) != 1 || updated.Phones[0] != "222" {
		t.Fatalf("unexpected phones: %#v", updated.Phones)
	}
	if updated.Title != "Visionary" {
		t.Fatalf("unexpected title: %q", updated.Title)
	}
}

func TestNormalizeProvidedContactVCardInjectsUID(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nFN:Grace Hopper\nEMAIL:grace@example.com\nEND:VCARD"
	normalized, err := normalizeProvidedContactVCard("contact-456", raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed := parseContactVCard(normalized)
	if parsed.UID != "contact-456" {
		t.Fatalf("unexpected uid: %q", parsed.UID)
	}
	if parsed.FullName != "Grace Hopper" {
		t.Fatalf("unexpected full name: %q", parsed.FullName)
	}
	if len(parsed.Emails) != 1 || parsed.Emails[0] != "grace@example.com" {
		t.Fatalf("unexpected emails: %#v", parsed.Emails)
	}
}

func TestContactToolArrayParametersDeclareItems(t *testing.T) {
	for _, tool := range []Tool{NewCreateContactTool(), NewModifyContactTool()} {
		params := tool.Parameters()
		for _, field := range []string{"emails", "phones"} {
			property, ok := params.Properties[field]
			if !ok {
				t.Fatalf("%s missing %s property", tool.Name(), field)
			}
			if property.Type != "array" {
				t.Fatalf("%s %s property type = %q, want array", tool.Name(), field, property.Type)
			}
			if property.Items == nil || property.Items.Type != "string" {
				t.Fatalf("%s %s property items = %#v, want string items", tool.Name(), field, property.Items)
			}
		}
	}
}