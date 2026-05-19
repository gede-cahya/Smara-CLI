package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RemoteDesktopDevice struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Token     string    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RemoteDesktopManager struct {
	path    string
	mu      sync.RWMutex
	devices map[string]RemoteDesktopDevice
}

func NewRemoteDesktopManager(path string) *RemoteDesktopManager {
	m := &RemoteDesktopManager{path: path, devices: map[string]RemoteDesktopDevice{}}
	_ = m.Load()
	return m
}

func (m *RemoteDesktopManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var list []RemoteDesktopDevice
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	for _, d := range list {
		m.devices[d.ID] = d
	}
	return nil
}

func (m *RemoteDesktopManager) Save() error {
	m.mu.RLock()
	list := make([]RemoteDesktopDevice, 0, len(m.devices))
	for _, d := range m.devices {
		list = append(list, d)
	}
	m.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(m.path, b, 0600)
}

func (m *RemoteDesktopManager) List() []RemoteDesktopDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]RemoteDesktopDevice, 0, len(m.devices))
	for _, d := range m.devices {
		if d.Token != "" {
			d.Token = "***"
		}
		list = append(list, d)
	}
	return list
}

func (m *RemoteDesktopManager) Upsert(name, rawURL, token string) (RemoteDesktopDevice, error) {
	name = strings.TrimSpace(name)
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if name == "" {
		name = "local-desktop"
	}
	if rawURL == "" {
		rawURL = "http://127.0.0.1:8765"
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return RemoteDesktopDevice{}, fmt.Errorf("url desktop-agent harus http/https")
	}
	id := strings.ToLower(strings.NewReplacer(" ", "-", "_", "-", "/", "-", ":", "-").Replace(name))
	now := time.Now()
	d := RemoteDesktopDevice{ID: id, Name: name, URL: rawURL, Token: token, CreatedAt: now, UpdatedAt: now}
	m.mu.Lock()
	if old, ok := m.devices[id]; ok {
		d.CreatedAt = old.CreatedAt
	}
	m.devices[id] = d
	m.mu.Unlock()
	return d, m.Save()
}

func (m *RemoteDesktopManager) Delete(id string) error {
	m.mu.Lock()
	delete(m.devices, id)
	m.mu.Unlock()
	return m.Save()
}

func (m *RemoteDesktopManager) Get(id string) (RemoteDesktopDevice, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[id]
	return d, ok
}

func (s *Server) remoteDesktopManager() *RemoteDesktopManager {
	if s.RemoteDesktop == nil {
		path := filepath.Join(os.TempDir(), "smara-remote-desktop-devices.json")
		if s.Cfg != nil && s.Cfg.DBPath != "" {
			path = filepath.Join(filepath.Dir(s.Cfg.DBPath), "remote-desktop-devices.json")
		}
		s.RemoteDesktop = NewRemoteDesktopManager(path)
	}
	return s.RemoteDesktop
}

func (s *Server) handleRemoteDesktopDevices(w http.ResponseWriter, r *http.Request) {
	m := s.remoteDesktopManager()
	switch r.Method {
	case http.MethodGet:
		jsonResponse(w, 200, map[string]interface{}{"devices": m.List()})
	case http.MethodPost:
		var in struct {
			Name, URL, Token string `json:"name,url,token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		d, err := m.Upsert(in.Name, in.URL, in.Token)
		if err != nil {
			errorResponse(w, 400, err.Error())
			return
		}
		if d.Token != "" {
			d.Token = "***"
		}
		jsonResponse(w, 200, d)
	default:
		errorResponse(w, 405, "method not allowed")
	}
}

func (s *Server) handleRemoteDesktopDeviceByID(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/remote-desktop/devices/"), "/")
	if id == "" {
		errorResponse(w, 400, "device id required")
		return
	}
	if r.Method != http.MethodDelete {
		errorResponse(w, 405, "only DELETE")
		return
	}
	if err := s.remoteDesktopManager().Delete(id); err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleRemoteDesktopProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, 405, "only POST")
		return
	}
	var in struct {
		DeviceID string                 `json:"device_id"`
		Action   string                 `json:"action"`
		Payload  map[string]interface{} `json:"payload"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.DeviceID == "" {
		in.DeviceID = "local-desktop"
	}
	d, ok := s.remoteDesktopManager().Get(in.DeviceID)
	if !ok {
		errorResponse(w, 404, "device belum dipair")
		return
	}
	path := map[string]string{"health": "/health", "observe": "/screenshot", "screenshot": "/screenshot", "stop": "/stop", "resume": "/resume", "task": "/task/run", "active_window": "/window/active"}[in.Action]
	if path == "" {
		errorResponse(w, 400, "action tidak didukung")
		return
	}
	method := http.MethodPost
	var body io.Reader
	if in.Action == "health" || in.Action == "observe" || in.Action == "screenshot" || in.Action == "active_window" {
		method = http.MethodGet
	} else {
		b, _ := json.Marshal(in.Payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, d.URL+path, body)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		errorResponse(w, 502, err.Error())
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func (s *Server) handleRemoteDesktopScreenshot(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("device_id")
	if id == "" {
		id = "local-desktop"
	}
	d, ok := s.remoteDesktopManager().Get(id)
	if !ok {
		errorResponse(w, 404, "device belum dipair")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, d.URL+"/screenshot.png", nil)
	if err != nil {
		errorResponse(w, 500, err.Error())
		return
	}
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		errorResponse(w, 502, err.Error())
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}
