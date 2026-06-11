package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const pdfTextMaxChars = 8000

type pdfTextTool struct {
	workspaceRoot string
}

// NewPdfTextTool creates a tool that extracts text from PDF files using pdftotext.
func NewPdfTextTool(workspaceRoot string) Tool {
	return &pdfTextTool{workspaceRoot: workspaceRoot}
}

func (t *pdfTextTool) Name() string { return "pdf_text" }

func (t *pdfTextTool) Description() string {
	return "Extract text content from a PDF file. Use this to read, search, or summarize PDF documents. Returns the first 8000 characters of extracted text."
}

func (t *pdfTextTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to the PDF file.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *pdfTextTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Path string `json:"path"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	filePath := resolvePath(params.Path, t.workspaceRoot)

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("file not found: %s", filePath))
		}
		return ErrorResult(fmt.Sprintf("stat error: %s", err))
	}
	if info.IsDir() {
		return ErrorResult(fmt.Sprintf("%s is a directory, not a PDF file", filePath))
	}
	if !strings.HasSuffix(strings.ToLower(filePath), ".pdf") {
		return ErrorResult(fmt.Sprintf("%s does not appear to be a PDF file", filePath))
	}

	pdftotextPath, err := exec.LookPath("pdftotext")
	if err != nil {
		return ErrorResult("pdftotext not found — install poppler-utils: sudo apt install poppler-utils")
	}

	cmd := exec.CommandContext(ctx, pdftotextPath, filePath, "-")
	output, err := cmd.Output()
	if err != nil {
		return ErrorResult(fmt.Sprintf("pdftotext failed: %s", err))
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return TextResult("(PDF contains no extractable text — it may be a scanned image)")
	}

	if len(text) > pdfTextMaxChars {
		text = text[:pdfTextMaxChars] + fmt.Sprintf("\n\n[truncated — showing first %d of %d characters]", pdfTextMaxChars, len(output))
	}

	return TextResult(text)
}
