package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	for _, name := range []string{"type", "status", "limit", "offset"} {
		if value := strings.TrimSpace(r.URL.Query().Get(name)); value != "" {
			query.Set(name, value)
		}
	}
	s.proxyBackupJSON(w, r, http.MethodGet, "/backups", query, nil, 30*time.Second)
}

func (s *Server) getBackup(w http.ResponseWriter, r *http.Request) {
	id, err := backupPathSegment(r.PathValue("id"), "backup id")
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	s.proxyBackupJSON(w, r, http.MethodGet, "/backups/"+id, nil, nil, 30*time.Second)
}

func (s *Server) backupFiles(w http.ResponseWriter, r *http.Request) {
	id, err := backupPathSegment(r.PathValue("id"), "backup id")
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	s.proxyBackupJSON(w, r, http.MethodGet, "/backups/"+id+"/files", nil, nil, 30*time.Second)
}

func (s *Server) downloadBackupFile(w http.ResponseWriter, r *http.Request) {
	id, err := backupPathSegment(r.PathValue("id"), "backup id")
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	filename, err := backupPathSegment(r.PathValue("filename"), "artifact filename")
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	s.proxyBackupDownload(w, r, "/backups/"+id+"/files/"+filename)
}

func (s *Server) runBackup(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	if r.Body != nil {
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if readErr != nil {
			problem(w, http.StatusBadRequest, "invalid backup request")
			return
		}
		if len(bytes.TrimSpace(body)) > 0 {
			var input struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(body, &input); err != nil {
				problem(w, http.StatusBadRequest, "invalid backup request: "+err.Error())
				return
			}
			if strings.TrimSpace(input.Type) != "" {
				kind = strings.ToUpper(strings.TrimSpace(input.Type))
			}
		}
	}
	if kind != "FULL" && kind != "INCREMENTAL" {
		problem(w, http.StatusUnprocessableEntity, "type must be FULL or INCREMENTAL")
		return
	}
	query := url.Values{"type": []string{kind}}
	if s.proxyBackupJSON(w, r, http.MethodPost, "/backup", query, nil, 15*time.Minute) {
		s.audit(r, "backup.run", "backup", kind, map[string]any{"type": kind})
	}
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	id, err := backupPathSegment(r.PathValue("id"), "backup id")
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	query := url.Values{"backupId": []string{id}}
	if s.proxyBackupJSON(w, r, http.MethodPost, "/restore/drill", query, nil, 15*time.Minute) {
		s.audit(r, "backup.restore_drill", "backup", id, map[string]any{"backupId": id})
	}
}

// proxyBackupJSON keeps the backup admin token on the server side. Browsers
// only receive the platform API response and never connect to backup-service
// directly.
func (s *Server) proxyBackupJSON(w http.ResponseWriter, r *http.Request, method, path string, query url.Values, body io.Reader, timeout time.Duration) bool {
	response, ok := s.callBackup(w, r, method, path, query, body, timeout)
	if !ok {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		s.backupUpstreamProblem(w, response)
		return false
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return true
}

func (s *Server) proxyBackupDownload(w http.ResponseWriter, r *http.Request, path string) {
	response, ok := s.callBackup(w, r, http.MethodGet, path, nil, nil, 15*time.Minute)
	if !ok {
		return
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		s.backupUpstreamProblem(w, response)
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "Content-Disposition", "X-Checksum-SHA256"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *Server) callBackup(w http.ResponseWriter, r *http.Request, method, path string, query url.Values, body io.Reader, timeout time.Duration) (*http.Response, bool) {
	if strings.TrimSpace(s.cfg.BackupURL) == "" {
		problem(w, http.StatusServiceUnavailable, "backup service is not configured")
		return nil, false
	}
	if strings.TrimSpace(s.cfg.BackupToken) == "" {
		problem(w, http.StatusServiceUnavailable, "backup service token is not configured")
		return nil, false
	}
	base, err := url.Parse(s.cfg.BackupURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		problem(w, http.StatusServiceUnavailable, "backup service URL is invalid")
		return nil, false
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	base.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(r.Context(), method, base.String(), body)
	if err != nil {
		problem(w, http.StatusBadGateway, "could not create backup service request")
		return nil, false
	}
	request.Header.Set("Authorization", "Bearer "+s.cfg.BackupToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		if r.Context().Err() != nil {
			problem(w, http.StatusGatewayTimeout, "backup service request timed out")
		} else {
			problem(w, http.StatusBadGateway, "backup service is unavailable")
		}
		return nil, false
	}
	return response, true
}

func (s *Server) backupUpstreamProblem(w http.ResponseWriter, response *http.Response) {
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	problem(w, http.StatusBadGateway, fmt.Sprintf("backup service returned HTTP %d", response.StatusCode))
}

func backupPathSegment(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return "", fmt.Errorf("invalid %s", label)
	}
	return url.PathEscape(value), nil
}
