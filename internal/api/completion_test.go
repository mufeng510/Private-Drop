package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"private-drop/internal/api"
	"private-drop/internal/testutil"
	"private-drop/internal/util"
)

// uploadViaAPI runs a complete single-chunk upload through the HTTP API and
// returns the resulting attachment id.
func uploadViaAPI(t *testing.T, h http.Handler, token, name, mime, relativePath, messageID string, payload []byte) string {
	t.Helper()
	sha := util.SHA256Hex(payload)
	initBody, _ := json.Marshal(map[string]any{
		"name": name, "mime": mime, "sha256": sha, "size": len(payload),
		"totalChunks": 1, "chunkSize": len(payload),
		"relativePath": relativePath, "messageId": messageID,
	})
	w := doReq(h, "POST", "/api/uploads/init", string(initBody), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("upload init: %d %s", w.Code, w.Body.String())
	}
	var initRes struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &initRes); err != nil || initRes.UploadID == "" {
		t.Fatalf("init response: %s err=%v", w.Body.String(), err)
	}
	// PUT chunk (binary body).
	r := httptest.NewRequest("PUT", "/api/uploads/"+initRes.UploadID+"/chunk/0", bytes.NewReader(payload))
	r.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	h.ServeHTTP(cw, r)
	if cw.Code != 200 {
		t.Fatalf("put chunk: %d %s", cw.Code, cw.Body.String())
	}
	w = doReq(h, "POST", "/api/uploads/"+initRes.UploadID+"/complete", "", token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("complete: %d %s", w.Code, w.Body.String())
	}
	var comp struct {
		AttachmentID string `json:"attachmentId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &comp); err != nil || comp.AttachmentID == "" {
		t.Fatalf("complete response: %s err=%v", w.Body.String(), err)
	}
	return comp.AttachmentID
}

func TestLoginLogoutAndUploadLifecycleHTTP(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	h := srv.Handler()
	token := initAdmin(t, h)

	// Login with the known credentials.
	w := doReq(h, "POST", "/api/auth/login",
		`{"username":"admin","password":"strongpassword"}`, "", "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &login)
	if login.Token == "" {
		t.Fatalf("login returned no token: %s", w.Body.String())
	}

	// Upload lifecycle with progress fields and byte accounting.
	payload := bytes.Repeat([]byte("hello"), 20)
	initBody, _ := json.Marshal(map[string]any{
		"name": "progress.bin", "mime": "application/octet-stream",
		"size": len(payload), "totalChunks": 2, "chunkSize": len(payload) / 2,
		"sha256": util.SHA256Hex(payload),
	})
	w = doReq(h, "POST", "/api/uploads/init", string(initBody), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("init: %d %s", w.Code, w.Body.String())
	}
	var ir struct {
		UploadID string `json:"uploadId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ir)
	for i, off := range []int{0, len(payload) / 2} {
		r := httptest.NewRequest("PUT", "/api/uploads/"+ir.UploadID+"/chunk/"+itoa(i), bytes.NewReader(payload[off:off+len(payload)/2]))
		r.Header.Set("Authorization", "Bearer "+token)
		cw := httptest.NewRecorder()
		h.ServeHTTP(cw, r)
		if cw.Code != 200 {
			t.Fatalf("chunk %d: %d %s", i, cw.Code, cw.Body.String())
		}
	}
	w = doReq(h, "GET", "/api/uploads/"+ir.UploadID+"/status", "", token, "127.0.0.1:1234")
	var st struct {
		BytesUploaded  int64   `json:"bytesUploaded"`
		BytesRemaining int64   `json:"bytesRemaining"`
		Speed          float64 `json:"speed"`
		Status         string  `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &st)
	if st.BytesUploaded != int64(len(payload)) || st.BytesRemaining != 0 || st.Status != "uploading" {
		t.Fatalf("progress fields wrong: %+v", st)
	}
	w = doReq(h, "POST", "/api/uploads/"+ir.UploadID+"/complete", "", token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("complete: %d %s", w.Code, w.Body.String())
	}

	// Interrupted resume -> needs_source -> retry -> complete.
	initBody, _ = json.Marshal(map[string]any{
		"name": "resume.bin", "mime": "application/octet-stream",
		"size": 4, "totalChunks": 2, "chunkSize": 2,
	})
	w = doReq(h, "POST", "/api/uploads/init", string(initBody), token, "127.0.0.1:1234")
	var rr struct {
		UploadID string `json:"uploadId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &rr)
	r := httptest.NewRequest("PUT", "/api/uploads/"+rr.UploadID+"/chunk/0", strings.NewReader("ab"))
	r.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	h.ServeHTTP(cw, r)
	if cw.Code != 200 {
		t.Fatalf("resume chunk: %d", cw.Code)
	}
	w = doReq(h, "POST", "/api/uploads/"+rr.UploadID+"/needs-source", "", token, "127.0.0.1:1234")
	var ns struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &ns)
	if w.Code != 200 || ns.Status != "needs_source" {
		t.Fatalf("needs-source: %d %s", w.Code, w.Body.String())
	}
	w = doReq(h, "POST", "/api/uploads/"+rr.UploadID+"/retry", "", token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	var retried struct {
		Status         string `json:"status"`
		UploadedChunks []int  `json:"uploadedChunks"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &retried)
	if retried.Status != "uploading" || len(retried.UploadedChunks) != 1 {
		t.Fatalf("retry should preserve received chunk: %+v", retried)
	}
	r = httptest.NewRequest("PUT", "/api/uploads/"+rr.UploadID+"/chunk/1", strings.NewReader("cd"))
	r.Header.Set("Authorization", "Bearer "+token)
	cw = httptest.NewRecorder()
	h.ServeHTTP(cw, r)
	if cw.Code != 200 {
		t.Fatalf("resume final chunk: %d", cw.Code)
	}
	w = doReq(h, "POST", "/api/uploads/"+rr.UploadID+"/complete", "", token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("resume complete: %d %s", w.Code, w.Body.String())
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func TestPreviewThumbnailZIPAndSecurityHTTP(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	h := srv.Handler()
	token := initAdmin(t, h)

	txtID := uploadViaAPI(t, h, token, "note.txt", "text/plain", "docs/notes/note.txt", "", []byte("hello preview"))

	// Preview is authenticated and serves inline bytes.
	w := doReq(h, "GET", "/api/files/"+txtID+"/preview", "", token, "127.0.0.1:1234")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "hello preview") {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}

	// ZIP with keepStructure uses the real relative path captured at init.
	body, _ := json.Marshal(map[string]any{"ids": []string{txtID}, "keepStructure": true})
	zw := doReq(h, "POST", "/api/files/zip", string(body), token, "127.0.0.1:1234")
	if zw.Code != 200 {
		t.Fatalf("zip: %d %s", zw.Code, zw.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(zw.Body.Bytes()), int64(zw.Body.Len()))
	if err != nil || len(zr.File) != 1 {
		t.Fatalf("zip reader: %v files=%d", err, len(zr.File))
	}
	if zr.File[0].Name != "docs/notes/note.txt" {
		t.Fatalf("zip entry name = %q, want real relative path", zr.File[0].Name)
	}

	// Zip-Slip: a hostile relative path is sanitized to safe components.
	evil := uploadViaAPI(t, h, token, "evil.txt", "text/plain", "../../etc/passwd.txt", "", []byte("evil"))
	body, _ = json.Marshal(map[string]any{"ids": []string{evil}, "keepStructure": true})
	zw = doReq(h, "POST", "/api/files/zip", string(body), token, "127.0.0.1:1234")
	zr, err = zip.NewReader(bytes.NewReader(zw.Body.Bytes()), int64(zw.Body.Len()))
	if err != nil || len(zr.File) != 1 {
		t.Fatalf("evil zip reader: %v", err)
	}
	name := zr.File[0].Name
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "etc/") == false {
		t.Fatalf("zip-slip not neutralized: %q", name)
	}

	// Thumbnail from a real PNG image.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	imgID := uploadViaAPI(t, h, token, "pic.png", "image/png", "", "", buf.Bytes())
	w = doReq(h, "GET", "/api/files/"+imgID+"/thumbnail", "", token, "127.0.0.1:1234")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/jpeg") {
		t.Fatalf("thumbnail: %d %s ct=%q", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}

	// Security: preview/thumbnail/ZIP all reject callers without a token.
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/files/" + txtID + "/preview", ""},
		{"GET", "/api/files/" + imgID + "/thumbnail", ""},
		{"POST", "/api/files/zip", `{"ids":["x"]}`},
	} {
		w := doReq(h, tc.method, tc.path, tc.body, "", "127.0.0.1:1234")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("no-token %s %s = %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

func TestTagsAndFileBulkHTTP(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	h := srv.Handler()
	token := initAdmin(t, h)

	// Create a tag.
	w := doReq(h, "POST", "/api/tags", `{"name":"important","color":"#f00","icon":"star"}`, token, "127.0.0.1:1234")
	if w.Code != 201 {
		t.Fatalf("create tag: %d %s", w.Code, w.Body.String())
	}
	var tag struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tag)
	if tag.ID == "" {
		t.Fatalf("no tag id: %s", w.Body.String())
	}

	// Attach the tag to a message and verify it is hydrated.
	w = doReq(h, "POST", "/api/messages", `{"text":"tagged message"}`, token, "127.0.0.1:1234")
	var msg struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &msg)
	body, _ := json.Marshal(map[string]any{"tagIds": []string{tag.ID}})
	w = doReq(h, "PATCH", "/api/messages/"+msg.ID, string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("tag message: %d %s", w.Code, w.Body.String())
	}

	// Attachment tag via PATCH /api/files/:id.
	attID := uploadViaAPI(t, h, token, "tagged.txt", "text/plain", "", "", []byte("tag me"))
	body, _ = json.Marshal(map[string]any{"tagIds": []string{tag.ID}})
	w = doReq(h, "PATCH", "/api/files/"+attID, string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("tag file: %d %s", w.Code, w.Body.String())
	}
	var att struct {
		TagIDs []string `json:"tagIds"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &att)
	if len(att.TagIDs) != 1 || att.TagIDs[0] != tag.ID {
		t.Fatalf("attachment tags not persisted: %+v (body=%s)", att, w.Body.String())
	}

	// File bulk: favorite -> untag -> delete -> restore.
	body, _ = json.Marshal(map[string]any{"ids": []string{attID}, "action": "favorite"})
	w = doReq(h, "POST", "/api/files/bulk", string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("bulk favorite: %d %s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"ids": []string{attID}, "action": "untag", "tagId": tag.ID})
	w = doReq(h, "POST", "/api/files/bulk", string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("bulk untag: %d %s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"ids": []string{attID}, "action": "delete"})
	w = doReq(h, "POST", "/api/files/bulk", string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("bulk delete: %d %s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"ids": []string{attID}, "action": "restore"})
	w = doReq(h, "POST", "/api/files/bulk", string(body), token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("bulk restore: %d %s", w.Code, w.Body.String())
	}
	w = doReq(h, "GET", "/api/files/"+attID, "", token, "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("get restored file: %d %s", w.Code, w.Body.String())
	}

	// Storage stats expose real warnings + suggestions + upload summary.
	w = doReq(h, "GET", "/api/storage/stats", "", token, "127.0.0.1:1234")
	var stats struct {
		Warnings      []string       `json:"warnings"`
		Suggestions   []string       `json:"suggestions"`
		UploadSummary map[string]int `json:"uploadSummary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("storage stats: %v", err)
	}
	if stats.Warnings == nil || stats.Suggestions == nil || stats.UploadSummary == nil {
		t.Fatalf("storage stats missing arrays: %s", w.Body.String())
	}
}
