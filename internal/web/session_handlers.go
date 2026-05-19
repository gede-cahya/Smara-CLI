package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type sessionCreateRequest struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}
type sessionRenameRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleWebSessions(w http.ResponseWriter, r *http.Request) {
	if s.WebSessions == nil {
		errorResponse(w, http.StatusServiceUnavailable, "web session manager belum aktif")
		return
	}
	switch r.Method {
	case http.MethodGet:
		includeArchived := r.URL.Query().Get("archived") == "1" || r.URL.Query().Get("archived") == "true"
		jsonResponse(w, http.StatusOK, map[string]interface{}{"sessions": s.WebSessions.List(includeArchived)})
	case http.MethodPost:
		var req sessionCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		jsonResponse(w, http.StatusOK, s.WebSessions.Create(req.Name, req.Mode))
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "only GET/POST")
	}
}

func (s *Server) handleWebSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.WebSessions == nil {
		errorResponse(w, http.StatusServiceUnavailable, "web session manager belum aktif")
		return
	}
	id, action := splitSessionPath(strings.TrimPrefix(r.URL.Path, "/api/web-sessions/"))
	if id == "" {
		errorResponse(w, http.StatusBadRequest, "session id required")
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			if sess, ok := s.WebSessions.Get(id); ok {
				jsonResponse(w, http.StatusOK, sess)
				return
			}
			errorResponse(w, http.StatusNotFound, "session tidak ditemukan")
		case http.MethodDelete:
			if err := s.WebSessions.Delete(id); err != nil {
				errorResponse(w, http.StatusNotFound, err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			errorResponse(w, http.StatusMethodNotAllowed, "only GET/DELETE")
		}
		return
	}
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "only POST")
		return
	}
	switch action {
	case "rename":
		var req sessionRenameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			errorResponse(w, http.StatusBadRequest, "name required")
			return
		}
		if err := s.WebSessions.Rename(id, req.Name); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "renamed"})
	case "archive":
		if err := s.WebSessions.Archive(id, true); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "archived"})
	case "unarchive":
		if err := s.WebSessions.Archive(id, false); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "unarchived"})
	case "cancel", "stop":
		if err := s.WebSessions.Cancel(id); err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "cancelled"})
	default:
		errorResponse(w, http.StatusNotFound, "unknown session action")
	}
}

func splitSessionPath(p string) (id, action string) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) > 0 {
		id = parts[0]
	}
	if len(parts) > 1 {
		action = parts[1]
	}
	return
}
