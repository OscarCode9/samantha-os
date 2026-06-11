package tools

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type inspectImageTool struct {
	workspaceRoot string
}

// NewInspectImageTool creates a tool that attaches a local image so the model
// can inspect it visually, including reading visible text.
func NewInspectImageTool(workspaceRoot string) Tool {
	return &inspectImageTool{workspaceRoot: workspaceRoot}
}

func (t *inspectImageTool) Name() string { return "inspect_image" }

func (t *inspectImageTool) Description() string {
	return "Attach a local image file so the model can inspect it visually. Use this to read text from images (OCR), describe screenshots, inspect documents, or answer questions about visual content."
}

func (t *inspectImageTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to an image file such as PNG, JPG, JPEG, WEBP, GIF, BMP, or SVG.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *inspectImageTool) Execute(ctx context.Context, arguments string) Result {
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
		return ErrorResult(fmt.Sprintf("%s is a directory, not an image file", filePath))
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("open error: %s", err))
	}
	defer file.Close()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read error: %s", err))
	}
	if n == 0 {
		return ErrorResult("image file is empty")
	}

	mimeType := detectImageMimeType(filePath, header[:n])
	if mimeType == "" {
		return ErrorResult(fmt.Sprintf("%s does not appear to be a supported image file", filePath))
	}

	return imageAttachmentResult(filePath, mimeType)
}

func detectImageMimeType(filePath string, header []byte) string {
	mimeType := http.DetectContentType(header)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return mimeType
	}

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	default:
		return ""
	}
}