package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Maximum upload size for both images (clipboard paste) and documents
// (file attach). 25 MiB covers most screenshots and PDFs while protecting
// the host from runaway uploads.
const maxUploadBytes = 25 << 20

// handleClipboardUpload accepts an image pasted in the browser (multipart
// form upload OR base64 dataURL JSON) and saves it under
// ~/.smara/clip-images/. Returns the local path so the front-end can inject
// `[image:/path]` into the next chat message — same convention used by the
// TUI's Ctrl+V flow and Telegram/Discord adapters.
//
// Two transport modes:
//
//	multipart/form-data    field "file"
//	application/json       {"data_url": "data:image/png;base64,...."}
//
// POST /api/clipboard/upload
func (s *Server) handleClipboardUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	dir, err := smaraClipImagesDir()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("siapkan direktori gagal: %v", err))
		return
	}

	var data []byte
	var ext string
	contentType := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			errorResponse(w, http.StatusBadRequest, fmt.Sprintf("parse multipart gagal: %v", err))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			errorResponse(w, http.StatusBadRequest, "field 'file' tidak ditemukan")
			return
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("baca file gagal: %v", err))
			return
		}
		ext = strings.ToLower(filepath.Ext(header.Filename))
		if ext == "" {
			ext = extFromMime(header.Header.Get("Content-Type"))
		}

	case strings.HasPrefix(contentType, "application/json"):
		var body struct {
			DataURL string `json:"data_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errorResponse(w, http.StatusBadRequest, fmt.Sprintf("decode json gagal: %v", err))
			return
		}
		mime, raw, ok := parseDataURL(body.DataURL)
		if !ok {
			errorResponse(w, http.StatusBadRequest, "data_url bukan dataURL base64 yang valid")
			return
		}
		data, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, fmt.Sprintf("decode base64 gagal: %v", err))
			return
		}
		ext = extFromMime(mime)

	default:
		errorResponse(w, http.StatusUnsupportedMediaType, "kirim multipart/form-data atau application/json")
		return
	}

	if len(data) < 64 {
		errorResponse(w, http.StatusBadRequest, "payload terlalu kecil — bukan gambar")
		return
	}
	if ext == "" {
		ext = ".png"
	}

	fname := fmt.Sprintf("clip-web-%d%s", time.Now().UnixNano(), ext)
	out := filepath.Join(dir, fname)
	if err := os.WriteFile(out, data, 0644); err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("simpan gagal: %v", err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"path":   out,
		"size":   len(data),
		"source": "web",
		"kind":   "image",
		"ref":    fmt.Sprintf("[image:%s]", out),
	})
}

// handleAttachmentUpload accepts any file (PDF, text, source code, etc.)
// and stores it under ~/.smara/clip-images/. Distinct from
// handleClipboardUpload because it preserves the original filename and
// returns the appropriate token (`[image:/path]` for images, `[file:/path]`
// for everything else) so the front-end can pick the right steer.
//
// POST /api/attachments/upload
//
//	multipart/form-data    field "file"
func (s *Server) handleAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		errorResponse(w, http.StatusUnsupportedMediaType, "kirim multipart/form-data")
		return
	}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		errorResponse(w, http.StatusBadRequest, fmt.Sprintf("parse multipart gagal: %v", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "field 'file' tidak ditemukan")
		return
	}
	defer file.Close()

	dir, err := smaraClipImagesDir()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("siapkan direktori gagal: %v", err))
		return
	}

	// Sanitize the original filename to a safe basename and strip any
	// directory traversal. We keep the extension (helps the agent's
	// read_file detect content type later).
	orig := filepath.Base(strings.TrimSpace(header.Filename))
	if orig == "" || orig == "." || orig == ".." {
		orig = "upload"
	}
	orig = sanitizeFilename(orig)
	stem := strings.TrimSuffix(orig, filepath.Ext(orig))
	ext := strings.ToLower(filepath.Ext(orig))
	if ext == "" {
		ext = extFromMime(header.Header.Get("Content-Type"))
	}
	fname := fmt.Sprintf("web-%d-%s%s", time.Now().UnixNano(), stem, ext)
	out := filepath.Join(dir, fname)

	dst, err := os.Create(out)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("buat file gagal: %v", err))
		return
	}
	defer dst.Close()

	n, err := io.Copy(dst, file)
	if err != nil {
		_ = os.Remove(out)
		errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("tulis file gagal: %v", err))
		return
	}
	if n == 0 {
		_ = os.Remove(out)
		errorResponse(w, http.StatusBadRequest, "file kosong")
		return
	}

	mime := header.Header.Get("Content-Type")
	kind := "file"
	ref := fmt.Sprintf("[file:%s]", out)
	if isImageMime(mime) || isImageExt(ext) {
		kind = "image"
		ref = fmt.Sprintf("[image:%s]", out)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"path":   out,
		"size":   n,
		"source": "web",
		"kind":   kind,
		"mime":   mime,
		"name":   header.Filename,
		"ref":    ref,
	})
}

// smaraClipImagesDir mirrors clipboard.SmaraTempImagePath without importing
// the TUI package (avoids dragging Bubble Tea into the web build path).
func smaraClipImagesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".smara", "clip-images")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func parseDataURL(s string) (mime, b64 string, ok bool) {
	// data:[<mime>][;base64],<data>
	if !strings.HasPrefix(s, "data:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(s, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", "", false
	}
	header := rest[:comma]
	payload := rest[comma+1:]
	if !strings.Contains(header, ";base64") {
		return "", "", false
	}
	mime = strings.TrimSuffix(header, ";base64")
	return mime, payload, true
}

func extFromMime(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "text/markdown":
		return ".md"
	case "application/json":
		return ".json"
	case "text/csv":
		return ".csv"
	}
	return ""
}

func isImageMime(m string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(m)), "image/")
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".svg":
		return true
	}
	return false
}

// sanitizeFilename strips characters that don't belong in a filename.
// We keep alphanumerics, dots, dashes and underscores; everything else
// becomes an underscore. Length is capped at 64 chars to keep the final
// path readable.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	if out == "" {
		out = "upload"
	}
	return out
}

// injectAttachmentSteer scans an incoming chat prompt for [image:/path]
// and [file:/path] tokens. When found, it appends a system-style note
// telling the agent which built-in tool to use so the model doesn't
// hallucinate the contents.
//
// Mirrors the behavior of the TUI's processImageRefs / processFileMentions
// without surfacing UI side-effects (the front-end already shows previews).
func looksLikeImageEditPrompt(lower string) bool {
	editSignals := []string{
		"ubah", "edit", "jadikan", "jadi", "transform", "convert", "konversi", "ganti", "modif", "modifikasi",
		"style", "stylize", "restyle", "retouch", "replace", "remove", "hapus", "tambahkan", "add",
		"kartun", "cartoon", "carton", "anime", "manga", "ghibli", "pixar", "disney", "vector", "vektor", "sketsa", "sketch",
	}
	for _, s := range editSignals {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func injectAttachmentSteer(prompt string) string {
	hasImage := strings.Contains(prompt, "[image:")
	hasFile := strings.Contains(prompt, "[file:")
	if !hasImage && !hasFile {
		return prompt
	}
	var hints []string
	if hasImage {
		lower := strings.ToLower(prompt)
		if looksLikeImageEditPrompt(lower) {
			hints = append(hints, "pesan ini terlihat seperti permintaan edit/ubah gambar atau image-to-image. Gunakan tool edit_image langsung dengan image_path dari token [image:/path] dan prompt edit user. Jangan pakai analyze_image hanya untuk style transfer dan jangan fallback ke generate_image tanpa input gambar")
		} else {
			hints = append(hints, "untuk gambar pakai tool analyze_image dengan path tersebut")
		}
	}
	if hasFile {
		hints = append(hints, "untuk dokumen (PDF/DOCX/TXT/dll) pakai tool read_document — JANGAN pakai read_file untuk file biner karena akan mengembalikan byte mentah dan merusak konteks")
	}
	steer := "\n\n[Sistem: pesan ini menyertakan lampiran. " +
		strings.Join(hints, "; ") +
		". Setelah dapat hasilnya, jawab user berdasarkan info aktual.]"
	return prompt + steer
}
