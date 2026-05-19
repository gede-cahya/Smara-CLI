package web

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gede-cahya/Smara-CLI/internal/avatar"
)

var avatarState = struct {
	sync.RWMutex
	cfg avatar.Config
}{cfg: avatar.DefaultConfig()}

func (s *Server) handleAvatarState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		avatarState.RLock()
		cfg := avatarState.cfg
		avatarState.RUnlock()
		jsonResponse(w, http.StatusOK, cfg)
	case http.MethodPost:
		var req avatar.Config
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			errorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		req.State = avatar.NormalizeState(req.State)
		if req.Expression == "" {
			req.Expression = avatar.ExpressionForState(req.State)
		}
		avatarState.Lock()
		avatarState.cfg = req
		avatarState.Unlock()
		jsonResponse(w, http.StatusOK, req)
	default:
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAvatarEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var ev avatar.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	avatarState.Lock()
	cfg := avatar.ApplyEvent(avatarState.cfg, ev)
	avatarState.cfg = cfg
	avatarState.Unlock()
	jsonResponse(w, http.StatusOK, cfg)
}
