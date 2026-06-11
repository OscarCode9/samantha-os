package tools

func imageAttachmentResult(path string, mimeType string) Result {
	result := JSONResult(map[string]any{
		"ok":        true,
		"path":      path,
		"mime_type": mimeType,
	})
	if !result.IsError {
		result.Attachments = []Attachment{{
			Type:     "image",
			Path:     path,
			MimeType: mimeType,
		}}
	}
	return result
}