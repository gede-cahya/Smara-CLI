package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gede-cahya/Smara-CLI/internal/avatar"
)

var avatarState = struct {
	sync.RWMutex
	cfg avatar.Config
}{cfg: avatar.DefaultConfig()}

const smaraAvatarModelPath = "/home/cahya/.local/share/Steam/steamapps/common/VRoid Studio/smaraAva.vrm"

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

func (s *Server) handleAvatarModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	info, err := os.Stat(smaraAvatarModelPath)
	if err != nil || info.IsDir() {
		errorResponse(w, http.StatusNotFound, "avatar model not found")
		return
	}
	w.Header().Set("Content-Type", "model/gltf-binary")
	w.Header().Set("Content-Disposition", `inline; filename="`+filepath.Base(smaraAvatarModelPath)+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, smaraAvatarModelPath)
}
