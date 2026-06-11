package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	edsAddressBookBus          = "org.gnome.evolution.dataserver.AddressBook10"
	edsAddressBookFactory      = "/org/gnome/evolution/dataserver/AddressBookFactory"
	edsAddressBookFactoryIface = "org.gnome.evolution.dataserver.AddressBookFactory"
	edsAddressBookIface        = "org.gnome.evolution.dataserver.AddressBook"
	edsAddressBookDefaultUID   = "system-address-book"
)

type contactRecord struct {
	UID          string   `json:"uid"`
	FullName     string   `json:"full_name"`
	GivenName    string   `json:"given_name,omitempty"`
	FamilyName   string   `json:"family_name,omitempty"`
	Nickname     string   `json:"nickname,omitempty"`
	Emails       []string `json:"emails,omitempty"`
	Phones       []string `json:"phones,omitempty"`
	Organization string   `json:"organization,omitempty"`
	Title        string   `json:"title,omitempty"`
	Note         string   `json:"note,omitempty"`
	VCard        string   `json:"vcard,omitempty"`
}

type contactMutation struct {
	UID          string   `json:"uid"`
	RawVCard     string   `json:"raw_vcard"`
	FullName     string   `json:"full_name"`
	GivenName    string   `json:"given_name"`
	FamilyName   string   `json:"family_name"`
	Nickname     string   `json:"nickname"`
	Email        string   `json:"email"`
	Emails       []string `json:"emails"`
	Phone        string   `json:"phone"`
	Phones       []string `json:"phones"`
	Organization string   `json:"organization"`
	Title        string   `json:"title"`
	Note         string   `json:"note"`
}

func generateContactUID() string {
	return "sam-contact-" + strings.TrimPrefix(generateCalendarUID(), "sam-")
}

func openSystemAddressBookClient(ctx context.Context, run commandRunner) (objectPath string, busName string, sourceUID string, err error) {
	if run == nil {
		run = defaultCommandRunner
	}

	candidates := []string{
		edsAddressBookDefaultUID,
		"system-addressbook",
		"personal",
		"contacts",
	}

	for _, uid := range candidates {
		if strings.TrimSpace(uid) == "" {
			continue
		}

		output, callErr := run(ctx,
			"busctl", "--user", "--json=short", "call",
			edsAddressBookBus,
			edsAddressBookFactory,
			edsAddressBookFactoryIface,
			"OpenAddressBook",
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
			return response.Data[0], response.Data[1], uid, nil
		}
	}

	return "", "", "", fmt.Errorf("unable to open address book source; tried %s", strings.Join(candidates, ", "))
}

func getAddressBookContactList(ctx context.Context, run commandRunner, query string) ([]string, string, error) {
	if run == nil {
		run = defaultCommandRunner
	}

	objectPath, busName, sourceUID, err := openSystemAddressBookClient(ctx, run)
	if err != nil {
		return nil, "", err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsAddressBookIface,
		"GetContactList",
		"s",
		query,
	)
	if callErr != nil {
		return nil, sourceUID, fmt.Errorf("get contact list failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string     `json:"type"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, sourceUID, fmt.Errorf("parse GetContactList response: %w", err)
	}
	if len(response.Data) == 0 {
		return []string{}, sourceUID, nil
	}
	return response.Data[0], sourceUID, nil
}

func getAddressBookContact(ctx context.Context, run commandRunner, uid string) (string, string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return "", "", fmt.Errorf("uid is required")
	}

	objectPath, busName, sourceUID, err := openSystemAddressBookClient(ctx, run)
	if err != nil {
		return "", "", err
	}

	output, callErr := run(ctx,
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsAddressBookIface,
		"GetContact",
		"s",
		uid,
	)
	if callErr != nil {
		return "", sourceUID, fmt.Errorf("get contact failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string   `json:"type"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", sourceUID, fmt.Errorf("parse GetContact response: %w", err)
	}
	if len(response.Data) == 0 {
		return "", sourceUID, fmt.Errorf("contact not found")
	}
	return response.Data[0], sourceUID, nil
}

func createAddressBookContacts(ctx context.Context, run commandRunner, vcards []string) ([]string, string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if len(vcards) == 0 {
		return nil, "", fmt.Errorf("at least one vcard is required")
	}

	objectPath, busName, sourceUID, err := openSystemAddressBookClient(ctx, run)
	if err != nil {
		return nil, "", err
	}

	args := []string{
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsAddressBookIface,
		"CreateContacts",
		"asu",
		fmt.Sprintf("%d", len(vcards)),
	}
	args = append(args, vcards...)
	args = append(args, "0")

	output, callErr := run(ctx, args[0], args[1:]...)
	if callErr != nil {
		return nil, sourceUID, fmt.Errorf("create contacts failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}

	var response struct {
		Type string     `json:"type"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, sourceUID, fmt.Errorf("parse CreateContacts response: %w", err)
	}
	if len(response.Data) == 0 {
		return []string{}, sourceUID, nil
	}
	return response.Data[0], sourceUID, nil
}

func modifyAddressBookContacts(ctx context.Context, run commandRunner, vcards []string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if len(vcards) == 0 {
		return "", fmt.Errorf("at least one vcard is required")
	}

	objectPath, busName, sourceUID, err := openSystemAddressBookClient(ctx, run)
	if err != nil {
		return "", err
	}

	args := []string{
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsAddressBookIface,
		"ModifyContacts",
		"asu",
		fmt.Sprintf("%d", len(vcards)),
	}
	args = append(args, vcards...)
	args = append(args, "0")

	output, callErr := run(ctx, args[0], args[1:]...)
	if callErr != nil {
		return sourceUID, fmt.Errorf("modify contacts failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return sourceUID, nil
}

func removeAddressBookContacts(ctx context.Context, run commandRunner, uids []string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}
	if len(uids) == 0 {
		return "", fmt.Errorf("at least one uid is required")
	}

	objectPath, busName, sourceUID, err := openSystemAddressBookClient(ctx, run)
	if err != nil {
		return "", err
	}

	args := []string{
		"busctl", "--user", "--json=short", "call",
		busName,
		objectPath,
		edsAddressBookIface,
		"RemoveContacts",
		"asu",
		fmt.Sprintf("%d", len(uids)),
	}
	args = append(args, uids...)
	args = append(args, "0")

	output, callErr := run(ctx, args[0], args[1:]...)
	if callErr != nil {
		return sourceUID, fmt.Errorf("remove contacts failed: %v (%s)", callErr, strings.TrimSpace(string(output)))
	}
	return sourceUID, nil
}

func escapeAddressBookQueryValue(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `"`, `\"`)
	return replacer.Replace(value)
}

func buildContactSearchQuery(term string) string {
	escaped := escapeAddressBookQueryValue(term)
	return fmt.Sprintf(`(or (contains "full_name" "%s") (contains "email" "%s"))`, escaped, escaped)
}

func cleanContactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeContactMutation(m contactMutation) contactMutation {
	m.UID = strings.TrimSpace(m.UID)
	m.RawVCard = strings.TrimSpace(m.RawVCard)
	m.FullName = strings.TrimSpace(m.FullName)
	m.GivenName = strings.TrimSpace(m.GivenName)
	m.FamilyName = strings.TrimSpace(m.FamilyName)
	m.Nickname = strings.TrimSpace(m.Nickname)
	m.Email = strings.TrimSpace(m.Email)
	m.Phone = strings.TrimSpace(m.Phone)
	m.Organization = strings.TrimSpace(m.Organization)
	m.Title = strings.TrimSpace(m.Title)
	m.Note = strings.TrimSpace(m.Note)
	m.Emails = cleanContactStrings(m.Emails)
	m.Phones = cleanContactStrings(m.Phones)
	if m.Email != "" && len(m.Emails) == 0 {
		m.Emails = []string{m.Email}
	}
	if m.Phone != "" && len(m.Phones) == 0 {
		m.Phones = []string{m.Phone}
	}
	return m
}

func contactDisplayName(record contactRecord) string {
	if strings.TrimSpace(record.FullName) != "" {
		return strings.TrimSpace(record.FullName)
	}
	joined := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(record.GivenName), strings.TrimSpace(record.FamilyName)}, " "))
	if joined != "" {
		return joined
	}
	if len(record.Emails) > 0 {
		return record.Emails[0]
	}
	return ""
}

func applyContactMutation(base contactRecord, m contactMutation) contactRecord {
	m = normalizeContactMutation(m)
	if m.UID != "" {
		base.UID = m.UID
	}
	if m.FullName != "" {
		base.FullName = m.FullName
	}
	if m.GivenName != "" {
		base.GivenName = m.GivenName
	}
	if m.FamilyName != "" {
		base.FamilyName = m.FamilyName
	}
	if m.Nickname != "" {
		base.Nickname = m.Nickname
	}
	if len(m.Emails) > 0 {
		base.Emails = m.Emails
	}
	if len(m.Phones) > 0 {
		base.Phones = m.Phones
	}
	if m.Organization != "" {
		base.Organization = m.Organization
	}
	if m.Title != "" {
		base.Title = m.Title
	}
	if m.Note != "" {
		base.Note = m.Note
	}
	if strings.TrimSpace(base.FullName) == "" {
		base.FullName = contactDisplayName(base)
	}
	base.Emails = cleanContactStrings(base.Emails)
	base.Phones = cleanContactStrings(base.Phones)
	return base
}

func escapeVCardValue(value string) string {
	return escapeICS(value)
}

func unescapeVCardValue(value string) string {
	replacer := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return replacer.Replace(value)
}

func unfoldVCardLines(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	baseLines := strings.Split(normalized, "\n")
	result := make([]string, 0, len(baseLines))
	for _, line := range baseLines {
		if len(result) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			result[len(result)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		result = append(result, line)
	}
	return result
}

func parseContactVCard(raw string) contactRecord {
	record := contactRecord{VCard: strings.TrimSpace(raw)}
	for _, line := range unfoldVCardLines(raw) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		keyPart := line[:idx]
		value := unescapeVCardValue(line[idx+1:])
		field := strings.ToUpper(strings.Split(keyPart, ";")[0])

		switch field {
		case "UID":
			record.UID = strings.TrimSpace(value)
		case "FN":
			record.FullName = strings.TrimSpace(value)
		case "N":
			parts := strings.Split(value, ";")
			if len(parts) > 0 {
				record.FamilyName = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				record.GivenName = strings.TrimSpace(parts[1])
			}
		case "NICKNAME":
			record.Nickname = strings.TrimSpace(value)
		case "EMAIL":
			record.Emails = append(record.Emails, strings.TrimSpace(value))
		case "TEL":
			record.Phones = append(record.Phones, strings.TrimSpace(value))
		case "ORG":
			record.Organization = strings.TrimSpace(value)
		case "TITLE":
			record.Title = strings.TrimSpace(value)
		case "NOTE":
			record.Note = strings.TrimSpace(value)
		}
	}
	record.Emails = cleanContactStrings(record.Emails)
	record.Phones = cleanContactStrings(record.Phones)
	if strings.TrimSpace(record.FullName) == "" {
		record.FullName = contactDisplayName(record)
	}
	return record
}

func buildContactVCard(record contactRecord) (string, error) {
	record.UID = strings.TrimSpace(record.UID)
	if record.UID == "" {
		record.UID = generateContactUID()
	}
	record.FullName = strings.TrimSpace(record.FullName)
	record.GivenName = strings.TrimSpace(record.GivenName)
	record.FamilyName = strings.TrimSpace(record.FamilyName)
	record.Nickname = strings.TrimSpace(record.Nickname)
	record.Organization = strings.TrimSpace(record.Organization)
	record.Title = strings.TrimSpace(record.Title)
	record.Note = strings.TrimSpace(record.Note)
	record.Emails = cleanContactStrings(record.Emails)
	record.Phones = cleanContactStrings(record.Phones)

	if record.FullName == "" {
		record.FullName = contactDisplayName(record)
	}
	if record.FullName == "" {
		return "", fmt.Errorf("full_name, given_name/family_name, or email is required to build a contact")
	}

	givenName := record.GivenName
	familyName := record.FamilyName
	if givenName == "" && familyName == "" {
		givenName = record.FullName
	}

	lines := []string{
		"BEGIN:VCARD",
		"VERSION:3.0",
		"UID:" + escapeVCardValue(record.UID),
		"FN:" + escapeVCardValue(record.FullName),
		"N:" + escapeVCardValue(familyName) + ";" + escapeVCardValue(givenName) + ";;;",
		"X-EVOLUTION-FILE-AS:" + escapeVCardValue(record.FullName),
	}
	if record.Nickname != "" {
		lines = append(lines, "NICKNAME:"+escapeVCardValue(record.Nickname))
	}
	for _, email := range record.Emails {
		lines = append(lines, "EMAIL;TYPE=INTERNET:"+escapeVCardValue(email))
	}
	for _, phone := range record.Phones {
		lines = append(lines, "TEL;TYPE=VOICE:"+escapeVCardValue(phone))
	}
	if record.Organization != "" {
		lines = append(lines, "ORG:"+escapeVCardValue(record.Organization))
	}
	if record.Title != "" {
		lines = append(lines, "TITLE:"+escapeVCardValue(record.Title))
	}
	if record.Note != "" {
		lines = append(lines, "NOTE:"+escapeVCardValue(record.Note))
	}
	lines = append(lines, "END:VCARD")
	return strings.Join(lines, "\r\n"), nil
}

func normalizeProvidedContactVCard(uid string, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("raw_vcard must not be empty")
	}
	upperRaw := strings.ToUpper(raw)
	if !strings.Contains(upperRaw, "BEGIN:VCARD") || !strings.Contains(upperRaw, "END:VCARD") {
		return "", fmt.Errorf("raw_vcard must be a complete VCARD with BEGIN:VCARD and END:VCARD")
	}

	record := parseContactVCard(raw)
	if strings.TrimSpace(uid) != "" {
		record.UID = strings.TrimSpace(uid)
	} else if strings.TrimSpace(record.UID) == "" {
		record.UID = generateContactUID()
	}

	normalized, err := buildContactVCard(record)
	if err != nil {
		return "", err
	}
	return normalized, nil
}

func contactRecordsFromVCards(vcards []string, includeVCard bool, limit int) []contactRecord {
	if limit <= 0 || limit > len(vcards) {
		limit = len(vcards)
	}
	contacts := make([]contactRecord, 0, limit)
	for i := 0; i < limit; i++ {
		record := parseContactVCard(vcards[i])
		if !includeVCard {
			record.VCard = ""
		}
		contacts = append(contacts, record)
	}
	return contacts
}