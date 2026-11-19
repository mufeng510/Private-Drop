package api

import (
	"errors"
	"net/http"
	"strconv"

	"private-drop/internal/upload"
)

func (s *Server) initUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string `json:"name"`
		Size         int64  `json:"size"`
		Mime         string `json:"mime"`
		SHA256       string `json:"sha256"`
		TotalChunks  int    `json:"totalChunks"`
		ChunkSize    int64  `json:"chunkSize"`
		MessageID    string `json:"messageId"`
		RelativePath string `json:"relativePath"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	p := principal(r)
	res, err := s.uploads.Init(r.Context(), p.DeviceID, upload.InitParams{
		Name: body.Name, Mime: body.Mime, SHA256: body.SHA256,
		MessageID: body.MessageID, RelativePath: body.RelativePath,
		Size: body.Size, TotalChunks: body.TotalChunks, ChunkSize: body.ChunkSize,
	})
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrUploadSize):
			writeErr(w, 413, "FILE_TOO_LARGE", "file exceeds the configured upload limit")
		case errors.Is(err, upload.ErrUploadConcurrency):
			writeErr(w, 429, "UPLOAD_CONCURRENCY", "too many active uploads; retry when another finishes")
		default:
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) putChunk(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		badRequest(w, "invalid chunk index")
		return
	}
	u, err := s.uploads.Chunk(r.Context(), r.PathValue("id"), idx, r.Body)
	if err != nil {
		if errors.Is(err, upload.ErrUploadNotFound) {
			notFound(w, "upload not found")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]any{"received": true, "uploadedChunks": len(u.UploadedChunks)})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	attID, err := s.uploads.Complete(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrUploadNotFound):
			notFound(w, "upload not found")
		case errors.Is(err, upload.ErrMessageUnavailable):
			writeErr(w, 409, "MESSAGE_UNAVAILABLE", err.Error())
		default:
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]any{"attachmentId": attID})
}

func (s *Server) uploadStatus(w http.ResponseWriter, r *http.Request) {
	u, err := s.uploads.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "upload not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, u)
}

func (s *Server) listUploads(w http.ResponseWriter, r *http.Request) {
	list, err := s.uploads.List(r.Context())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) cancelUpload(w http.ResponseWriter, r *http.Request) {
	if err := s.uploads.Cancel(r.Context(), r.PathValue("id")); err != nil {
		if isNotFound(err) {
			notFound(w, "upload not found")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) retryUpload(w http.ResponseWriter, r *http.Request) {
	u, err := s.uploads.Retry(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "upload not found")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, u)
}

func (s *Server) uploadNeedsSource(w http.ResponseWriter, r *http.Request) {
	u, err := s.uploads.MarkNeedsSource(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "upload not found")
		} else {
			badRequest(w, err.Error())
		}
		return
	}
	writeJSON(w, 200, u)
}
