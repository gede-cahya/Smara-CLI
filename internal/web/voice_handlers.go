package web

import (
	"encoding/json"
	"net/http"

	"github.com/gede-cahya/Smara-CLI/internal/voice"
)

func (s *Server) handleVoiceSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jsonResponse(w, http.StatusOK, voice.DefaultSettings())
}

func (s *Server) handleVoiceCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req voice.CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, voice.PlanCommand(req))
}

func (s *Server) handleVoiceSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req voice.SynthesisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, voice.Synthesize(r.Context(), req))
}
