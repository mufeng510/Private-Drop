package api

import (
	"net/http"

	"private-drop/internal/message"
)

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	msgs, next, err := s.messages.List(r.Context(), message.MessageFilter{
		Cursor:         q.Get("cursor"),
		Limit:          queryInt(r, "limit", 50),
		Sort:           q.Get("sort"),
		Type:           q.Get("type"),
		Tag:            q.Get("tag"),
		From:           q.Get("from"),
		To:             q.Get("to"),
		Favorite:       queryBool(r, "favorite"),
		IncludeDeleted: false,
	})
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": msgs, "nextCursor": next})
}

func (s *Server) createMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text          string   `json:"text"`
		AttachmentIDs []string `json:"attachmentIds"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	p := principal(r)
	m, err := s.messages.Create(r.Context(), p.UserID, p.DeviceID, body.Text, body.AttachmentIDs)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 201, m)
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	m, err := s.messages.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if isNotFound(err) {
			notFound(w, "message not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) updateMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      *string   `json:"text"`
		Pinned    *bool     `json:"pinned"`
		Favorited *bool     `json:"favorited"`
		TagIDs    *[]string `json:"tagIds"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	m, err := s.messages.Update(r.Context(), r.PathValue("id"), body.Text, body.Pinned, body.Favorited, body.TagIDs)
	if err != nil {
		if isNotFound(err) {
			notFound(w, "message not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, m)
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	if err := s.messages.SoftDelete(r.Context(), r.PathValue("id")); err != nil {
		if isNotFound(err) {
			notFound(w, "message not found")
		} else {
			serverErr(w, err)
		}
		return
	}
	writeJSON(w, 200, map[string]any{})
}

func (s *Server) bulkMessages(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		TagID  string   `json:"tagId"`
		Mode   string   `json:"mode"`
	}
	if err := decodeBody(r, &body); err != nil {
		badRequest(w, "invalid request body")
		return
	}
	res, err := s.messages.Bulk(r.Context(), body.IDs, body.Action, body.TagID, body.Mode, s.clientIP(r))
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) expiringMessages(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 7)
	list, err := s.messages.Expiring(r.Context(), days)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": list})
}
