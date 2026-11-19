package api

import (
	"errors"
	"net/http"

	"private-drop/internal/file"
)

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, next, err := s.files.List(r.Context(), file.FileFilter{
		Type: q.Get("type"), Tag: q.Get("tag"), Sort: q.Get("sort"),
		Favorite: queryBool(r, "favorite"), From: q.Get("from"), To: q.Get("to"),
		Cursor: q.Get("cursor"), Limit: queryInt(r, "limit", 100),
	})
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list, "nextCursor": next})
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	a, err := s.files.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) updateFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Favorited *bool     `json:"favorited"`
		Name      *string   `json:"name"`
		TagIDs    *[]string `json:"tagIds"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	a, err := s.files.Update(r.Context(), r.PathValue("id"), body.Favorited, body.Name, body.TagIDs)
	if err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) bulkFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		TagID  string   `json:"tagId"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	affected, err := s.files.Bulk(r.Context(), body.IDs, body.Action, body.TagID)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"affected": affected})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if err := s.files.SoftDelete(r.Context(), r.PathValue("id")); err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	f, a, err := s.files.Open(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	if err := s.files.Serve(w, r, f, a, false); err != nil {
		// Headers may already be written; just log.
		_ = err
	}
}

func (s *Server) previewFile(w http.ResponseWriter, r *http.Request) {
	f, a, err := s.files.Open(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	if err := s.files.Serve(w, r, f, a, true); err != nil {
		_ = err
	}
}

func (s *Server) thumbnailFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.files.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			notFound(w, "file not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	path, err := s.files.EntityPath(r.Context(), id)
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := s.files.WriteThumbnail(w, a, path); err != nil {
		if errors.Is(err, file.ErrNoThumbnail) {
			writeErr(w, 404, "THUMBNAIL_UNAVAILABLE", "no thumbnail for this file type")
		} else {
			writeErr(w, 404, "THUMBNAIL_UNAVAILABLE", "thumbnail generation failed")
		}
	}
}

func (s *Server) zipFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs           []string `json:"ids"`
		KeepStructure bool     `json:"keepStructure"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	if len(body.IDs) == 0 {
		badRequest(w, "no files selected")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"private-drop.zip\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := s.files.WriteZIP(r.Context(), w, body.IDs, body.KeepStructure); err != nil {
		_ = err // stream already started
	}
}
