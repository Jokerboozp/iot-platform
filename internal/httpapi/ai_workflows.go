package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"iot-platform/internal/core"
	"iot-platform/internal/model"
	"iot-platform/internal/parser"
)

func (s *Server) runAIAlarmAnalysis(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.engine.AnalyzeAlarm(r.Context(), claims(r).TenantID, r.PathValue("alarmId"))
	if err != nil {
		problem(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "ai.alarm-analysis.run", "alarm", analysis.AlarmID, map[string]any{"manual": true})
	write(w, http.StatusOK, analysis)
}

func (s *Server) healthInspection(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	report, err := s.engine.InspectDeviceHealth(ctx, claims(r).TenantID)
	if err != nil {
		problem(w, http.StatusBadGateway, err.Error())
		return
	}
	write(w, http.StatusOK, report)
}

func (s *Server) generateProtocolAssistant(w http.ResponseWriter, r *http.Request) {
	const maxDocumentBytes = 32 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			problem(w, http.StatusRequestEntityTooLarge, "protocol document exceeds 32 MiB")
			return
		}
		problem(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	documentText, err := readProtocolAssistantDocument(r, maxDocumentBytes)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	pointTable := strings.TrimSpace(r.FormValue("pointTable"))
	if documentText == "" && pointTable == "" {
		problem(w, http.StatusUnprocessableEntity, "protocol document or point table is required")
		return
	}
	input := core.ProtocolAssistantInput{
		Name:          strings.TrimSpace(r.FormValue("name")),
		Protocol:      strings.TrimSpace(r.FormValue("protocol")),
		Transport:     strings.TrimSpace(r.FormValue("transport")),
		PayloadFormat: strings.TrimSpace(r.FormValue("payloadFormat")),
		DocumentText:  documentText,
		PointTable:    pointTable,
		SamplePayload: strings.TrimSpace(r.FormValue("samplePayload")),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	draft, err := s.engine.GenerateProtocolAssistant(ctx, claims(r).TenantID, input)
	if err != nil {
		problem(w, http.StatusBadGateway, err.Error())
		return
	}
	s.audit(r, "ai.protocol-assistant.generate", "protocol-draft", "draft", map[string]any{"filename": protocolAssistantFilename(r), "fields": len(draft.Fields), "payloadFormat": draft.PayloadFormat})
	write(w, http.StatusOK, draft)
}

func (s *Server) previewProtocolAssistant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Draft         model.ProtocolAssistantDraft `json:"draft"`
		Source        string                       `json:"source"`
		Payload       json.RawMessage              `json:"payload"`
		PayloadFormat string                       `json:"payloadFormat"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	draft := in.Draft
	if strings.TrimSpace(in.Source) != "" {
		draft.Source = in.Source
	}
	if strings.TrimSpace(in.PayloadFormat) != "" {
		draft.PayloadFormat = strings.TrimSpace(in.PayloadFormat)
	}
	payload, err := assistantPayloadText(in.Payload, draft.PayloadFormat)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	message, err := core.PreviewProtocolAssistant(draft, claims(r).TenantID, payload)
	if err != nil {
		write(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	write(w, http.StatusOK, map[string]any{"success": true, "standardMessage": message})
}

func (s *Server) publishProtocolAssistant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID            string                       `json:"id"`
		Version       string                       `json:"version"`
		Status        string                       `json:"status"`
		Source        string                       `json:"source"`
		Payload       json.RawMessage              `json:"payload"`
		PayloadFormat string                       `json:"payloadFormat"`
		Draft         model.ProtocolAssistantDraft `json:"draft"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	draft := in.Draft
	if strings.TrimSpace(in.Source) != "" {
		draft.Source = in.Source
	}
	if strings.TrimSpace(in.PayloadFormat) != "" {
		draft.PayloadFormat = strings.TrimSpace(in.PayloadFormat)
	}
	payload, err := assistantPayloadText(in.Payload, draft.PayloadFormat)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	message, err := core.PreviewProtocolAssistant(draft, claims(r).TenantID, payload)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "发布前解析校验失败："+err.Error())
		return
	}
	source := strings.TrimSpace(draft.Source)
	if source == "" {
		source, err = core.BuildProtocolJavaScriptSource(draft)
		if err != nil {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	if _, err = parser.JavaScriptSource(map[string]any{"source": source}); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = "protocol_ai_" + randomHex(6)
	}
	if len(id) > 128 || strings.ContainsAny(id, "/\\") {
		problem(w, http.StatusUnprocessableEntity, "invalid protocol package id")
		return
	}
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	if status == "" {
		status = "PUBLISHED"
	}
	if status != "DRAFT" && status != "PUBLISHED" {
		problem(w, http.StatusUnprocessableEntity, "status must be DRAFT or PUBLISHED")
		return
	}
	now := time.Now().UnixMilli()
	pkg := model.ProtocolPackage{ID: id, TenantID: claims(r).TenantID, Name: draft.Name, Version: in.Version, Protocol: draft.Protocol, Transport: draft.Transport, PayloadFormat: draft.PayloadFormat, ParserType: parser.JavaScriptParserName, Status: status, Description: draft.Description, Config: map[string]any{"source": source}, CreatedAt: now, UpdatedAt: now}
	if pkg.Name == "" {
		pkg.Name = "AI 协议解析包"
	}
	if pkg.Version == "" {
		pkg.Version = "1.0.0"
	}
	if pkg.Protocol == "" {
		pkg.Protocol = "custom-javascript"
	}
	if pkg.Transport == "" {
		pkg.Transport = "MQTT"
	}
	if pkg.PayloadFormat == "" {
		pkg.PayloadFormat = "json"
	}
	if old, getErr := s.engine.Repo.GetProtocolPackage(r.Context(), pkg.TenantID, pkg.ID); getErr == nil {
		pkg.CreatedAt = old.CreatedAt
	}
	if err = s.engine.Repo.SaveProtocolPackage(r.Context(), pkg); err != nil {
		problem(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ai.protocol-assistant.publish", "protocolPackage", pkg.ID, map[string]any{"version": pkg.Version, "status": pkg.Status, "fields": len(draft.Fields)})
	write(w, http.StatusCreated, map[string]any{"package": pkg, "standardMessage": message})
}

func readProtocolAssistantDocument(r *http.Request, maximum int) (string, error) {
	f, header, err := r.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		return "", fmt.Errorf("read protocol document: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(maximum)+1))
	if err != nil {
		return "", err
	}
	if len(data) > maximum {
		return "", errors.New("protocol document exceeds 32 MiB")
	}
	text, err := core.ExtractKnowledgeText(header.Filename, data)
	if err != nil {
		return "", fmt.Errorf("extract protocol document: %w", err)
	}
	return text, nil
}

func protocolAssistantFilename(r *http.Request) string {
	if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) == 0 {
		return ""
	}
	return r.MultipartForm.File["file"][0].Filename
}

func assistantPayloadText(raw json.RawMessage, format string) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("sample payload is required")
	}
	if strings.EqualFold(format, "hex") {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", errors.New("hex sample payload must be a JSON string")
		}
		return value, nil
	}
	if !json.Valid(raw) {
		return "", errors.New("JSON sample payload is invalid")
	}
	return string(raw), nil
}
