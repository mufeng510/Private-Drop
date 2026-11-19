package api

import (
	"errors"
	"net/http"

	"private-drop/internal/auth"
	"private-drop/internal/file"
	"private-drop/internal/message"
	"private-drop/internal/upload"
)

func decodeBody(r *http.Request, dst any) error { return auth.DecodeJSON(r, dst) }

func isNotFound(err error) bool {
	return errors.Is(err, message.ErrMessageNotFound) ||
		errors.Is(err, file.ErrAttachmentNotFound) ||
		errors.Is(err, upload.ErrUploadNotFound) ||
		errors.Is(err, auth.ErrDeviceNotFound) ||
		errors.Is(err, auth.ErrRequestNotFound)
}
