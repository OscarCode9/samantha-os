package prompt

import "fmt"

const (
	// DefaultMaxCharsPerFile is the maximum characters kept from a single
	// workspace file before truncation.
	DefaultMaxCharsPerFile = 4000
	// DefaultTotalMaxChars is the total character budget across all injected
	// workspace files.
	DefaultTotalMaxChars = 20000
)

// WorkspaceFile represents a file to inject into the system prompt.
type WorkspaceFile struct {
	Name    string // e.g. "IDENTITY.md"
	Path    string // absolute path on disk
	Content string // file content (already read)
}

// TruncateWithBudget returns content truncated to maxChars. If truncation
// happens a marker is appended showing how much was cut.
func TruncateWithBudget(content string, maxChars int) string {
	if maxChars <= 0 || len(content) <= maxChars {
		return content
	}
	remaining := len(content) - maxChars
	return content[:maxChars] + fmt.Sprintf("\n[...truncated, %d chars remaining]", remaining)
}

// InjectWorkspaceFiles formats workspace files into tagged sections suitable
// for inclusion in the system prompt.  Each file is wrapped in XML-style
// boundary tags and individually capped at perFileBudget characters.  The
// total output is capped at totalBudget characters.
func InjectWorkspaceFiles(files []WorkspaceFile, perFileBudget, totalBudget int) string {
	if len(files) == 0 {
		return ""
	}
	if perFileBudget <= 0 {
		perFileBudget = DefaultMaxCharsPerFile
	}
	if totalBudget <= 0 {
		totalBudget = DefaultTotalMaxChars
	}

	var parts []string
	totalUsed := 0

	for _, f := range files {
		if f.Content == "" {
			continue
		}
		content := TruncateWithBudget(f.Content, perFileBudget)

		// Check whether adding this file would exceed the total budget.
		overhead := len("<workspace_file path=\"\">\n\n</workspace_file>") + len(f.Name)
		needed := overhead + len(content)
		if totalUsed+needed > totalBudget {
			remaining := totalBudget - totalUsed - overhead
			if remaining <= 0 {
				break
			}
			content = TruncateWithBudget(content, remaining)
			needed = overhead + len(content)
		}

		part := fmt.Sprintf("<workspace_file path=%q>\n%s\n</workspace_file>", f.Name, content)
		parts = append(parts, part)
		totalUsed += needed
	}

	if len(parts) == 0 {
		return ""
	}

	header := "## Project Context\n\nThese user-editable workspace files define your identity and behaviour:\n\n"
	return header + joinParts(parts)
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n\n"
		}
		result += p
	}
	return result
}
