package file

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrNoThumbnail = errors.New("thumbnail unavailable for this file type")

// dangerousInline lists MIME types that must never be served inline because
// they can execute script in the app's origin (XSS).
func dangerousInline(mimeType string) bool {
	m := strings.ToLower(mimeType)
	return strings.Contains(m, "html") || strings.Contains(m, "svg") || strings.Contains(m, "javascript") ||
		strings.Contains(m, "x-html")
}

// Serve streams an entity file with support for HTTP range requests. When
// inline is true it is served for preview; HTML/SVG are forced to attachment
// to avoid script execution in the application origin.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, f *os.File, a Attachment, inline bool) error {
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	ct := a.Mime
	if ct == "" || strings.Contains(ct, "\n") || strings.Contains(ct, "\r") {
		ct = "application/octet-stream"
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", ct)
	if inline && !dangerousInline(ct) {
		w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlPathEscape(a.Name))
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlPathEscape(a.Name))
	}
	http.ServeContent(w, r, a.Name, st.ModTime(), f)
	return nil
}

func urlPathEscape(s string) string {
	// A conservative RFC 5987 encoder sufficient for Content-Disposition.
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}

// WriteThumbnail generates (and caches) a small JPEG thumbnail for images.
// Other types return ErrNoThumbnail so callers can fall back gracefully.
func (s *Service) WriteThumbnail(w http.ResponseWriter, a Attachment, path string) error {
	if a.Type != "image" {
		return ErrNoThumbnail
	}
	thumbDir := filepath.Join(s.cfg.FilesDir(), "thumbs")
	cache := filepath.Join(thumbDir, a.ID+".jpg")
	if st, err := os.Stat(cache); err == nil && st.Size() > 0 {
		return writeFileResponse(w, cache, "image/jpeg")
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(src)
	src.Close()
	if err != nil {
		return ErrNoThumbnail
	}
	const maxDim = 320
	bounds := img.Bounds()
	w0, h0 := bounds.Dx(), bounds.Dy()
	if w0 <= 0 || h0 <= 0 {
		return ErrNoThumbnail
	}
	scale := float64(maxDim) / float64(max(w0, h0))
	if scale > 1 {
		scale = 1
	}
	nw, nh := max(1, int(float64(w0)*scale)), max(1, int(float64(h0)*scale))
	thumb := resizeImage(img, nw, nh)
	if err := os.MkdirAll(thumbDir, 0o750); err != nil {
		return err
	}
	tmp := filepath.Join(s.cfg.TmpDir(), "thumb-"+a.ID)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := encodeJPEG(out, thumb, 80); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, cache); err != nil {
		os.Remove(tmp)
		return err
	}
	return writeFileResponse(w, cache, "image/jpeg")
}

func writeFileResponse(w http.ResponseWriter, path, contentType string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, err = w.Write(b)
	return err
}

// resizeImage performs a simple nearest-neighbour scale into an RGBA image.
func resizeImage(src image.Image, nw, nh int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for y := 0; y < nh; y++ {
		sy := sb.Min.Y + (y*sh)/nh
		for x := 0; x < nw; x++ {
			sx := sb.Min.X + (x*sw)/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func encodeJPEG(w io.Writer, img image.Image, quality int) error {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// WriteZIP streams a ZIP archive of the requested attachments. Entry names are
// sanitized and de-duplicated to prevent path traversal and overwrites.
func (s *Service) WriteZIP(ctx context.Context, w http.ResponseWriter, ids []string, keepStructure bool) error {
	zw := zip.NewWriter(w)
	defer zw.Close()
	used := map[string]bool{}
	for _, id := range ids {
		a, err := s.Get(ctx, id)
		if err != nil {
			continue // skip missing attachments rather than failing the whole archive
		}
		path, err := s.EntityPath(ctx, id)
		if err != nil {
			continue
		}
		name := a.Name
		if keepStructure {
			// Use the real relative path captured at upload init. It is still
			// passed through sanitizeZipName to block Zip-Slip ("../", absolute
			// paths, Windows drive prefixes) even though it came from the server.
			if a.OriginalPath != "" {
				name = a.OriginalPath
			}
		}
		name = sanitizeZipName(name)
		if used[name] {
			ext := filepath.Ext(name)
			name = strings.TrimSuffix(name, ext) + "-" + strconv.Itoa(len(used)) + ext
		}
		used[name] = true

		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		hdr, err := zip.FileInfoHeader(st)
		if err != nil {
			continue
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		entry, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		_, err = io.Copy(entry, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func sanitizeZipName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimLeft(name, "/")
	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for i, p := range parts {
		if p == "" || p == "." || p == ".." {
			continue
		}
		// Strip a Windows drive-letter prefix (e.g. "C:") and other leading
		// colon-bearing segments that could confuse extractors.
		if i == 0 && len(p) == 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "file"
	}
	return strings.Join(clean, "/")
}
