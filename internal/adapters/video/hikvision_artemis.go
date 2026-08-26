package video

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"iot-platform/internal/model"
)

const hikvisionPreviewPath = "/artemis/api/video/v2/cameras/previewURLs"

// HikvisionArtemisConfig contains the credentials for HikCentral Professional
// OpenAPI. The credentials belong to the platform deployment, not to a camera
// row, and must be injected from a secret store or environment.
type HikvisionArtemisConfig struct {
	BaseURL    string
	AppKey     string
	AppSecret  string
	HTTPClient *http.Client
	StreamType int
	Protocol   string
	Transmode  int
	Expand     string
}

// HikvisionArtemis is an in-process Go client for the official Hikvision
// Artemis OpenAPI. It signs the previewURLs request directly; it does not call
// a Java bridge, a local compatibility service, or a generic SDK sidecar.
type HikvisionArtemis struct {
	baseURL    string
	appKey     string
	appSecret  string
	client     *http.Client
	streamType int
	protocol   string
	transmode  int
	expand     string
}

func NewHikvisionArtemis(config HikvisionArtemisConfig) (*HikvisionArtemis, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if _, err := absoluteHTTPURL(baseURL); err != nil {
		return nil, fmt.Errorf("invalid Hikvision Artemis API URL: %w", err)
	}
	if strings.TrimSpace(config.AppKey) == "" || strings.TrimSpace(config.AppSecret) == "" {
		return nil, errors.New("Hikvision Artemis app key and app secret are required")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	protocol := strings.ToLower(strings.TrimSpace(config.Protocol))
	if protocol == "" {
		protocol = "rtsp"
	}
	if protocol != "rtsp" && protocol != "rtmp" && protocol != "hls" {
		return nil, fmt.Errorf("unsupported Hikvision preview protocol %q", protocol)
	}
	streamType := config.StreamType
	if streamType < 0 || streamType > 2 {
		return nil, errors.New("Hikvision stream type must be 0, 1, or 2")
	}
	transmode := config.Transmode
	if transmode == 0 {
		// TCP is the safe/default transport for server-side preview. Keep the
		// zero value useful for callers that only configure the endpoint and
		// credentials.
		transmode = 1
	}
	if transmode != 1 {
		return nil, errors.New("Hikvision transmode must be 1 (TCP)")
	}
	return &HikvisionArtemis{
		baseURL:    baseURL,
		appKey:     strings.TrimSpace(config.AppKey),
		appSecret:  strings.TrimSpace(config.AppSecret),
		client:     client,
		streamType: streamType,
		protocol:   protocol,
		transmode:  transmode,
		expand:     strings.TrimSpace(config.Expand),
	}, nil
}

func (c *HikvisionArtemis) Resolve(ctx context.Context, request StreamRequest) (model.VideoStream, error) {
	if c == nil {
		return model.VideoStream{}, errors.New("Hikvision Artemis adapter is nil")
	}
	if strings.TrimSpace(request.CameraID) == "" {
		return model.VideoStream{}, errors.New("Hikvision camera index code is required")
	}
	endpoint := c.baseURL
	if strings.TrimSpace(request.Endpoint) != "" {
		endpoint = strings.TrimRight(strings.TrimSpace(request.Endpoint), "/")
	}
	requestURL, err := hikvisionPreviewURL(endpoint)
	if err != nil {
		return model.VideoStream{}, err
	}
	body, err := json.Marshal(struct {
		CameraIndexCode string `json:"cameraIndexCode"`
		StreamType      int    `json:"streamType"`
		Protocol        string `json:"protocol"`
		Transmode       int    `json:"transmode"`
		Expand          string `json:"expand,omitempty"`
	}{
		CameraIndexCode: request.CameraID,
		StreamType:      c.streamType,
		Protocol:        c.protocol,
		Transmode:       c.transmode,
		Expand:          c.expand,
	})
	if err != nil {
		return model.VideoStream{}, fmt.Errorf("encode Hikvision preview request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return model.VideoStream{}, fmt.Errorf("create Hikvision preview request: %w", err)
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	applyHikvisionSignature(req, body, c.appKey, c.appSecret, date)
	resp, err := c.client.Do(req)
	if err != nil {
		return model.VideoStream{}, fmt.Errorf("request Hikvision Artemis preview URL: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSDKResponse+1))
	if err != nil {
		return model.VideoStream{}, fmt.Errorf("read Hikvision Artemis response: %w", err)
	}
	if len(responseBody) > maxSDKResponse {
		return model.VideoStream{}, errors.New("Hikvision Artemis response is too large")
	}
	if resp.StatusCode/100 != 2 {
		return model.VideoStream{}, fmt.Errorf("Hikvision Artemis returned HTTP %d", resp.StatusCode)
	}
	stream, err := decodeHikvisionPreviewResponse(responseBody)
	if err != nil {
		return model.VideoStream{}, err
	}
	return stream, nil
}

func hikvisionPreviewURL(raw string) (string, error) {
	u, err := absoluteHTTPURL(raw)
	if err != nil {
		return "", fmt.Errorf("invalid Hikvision Artemis endpoint: %w", err)
	}
	if u.User != nil {
		return "", errors.New("Hikvision Artemis endpoint userinfo is not allowed")
	}
	if !strings.Contains(u.Path, "/artemis/api/") {
		u.Path = path.Join(u.Path, hikvisionPreviewPath)
	}
	return u.String(), nil
}

func applyHikvisionSignature(req *http.Request, body []byte, appKey, appSecret, date string) {
	contentMD5 := md5.Sum(body)
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		copy(nonceBytes, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	const signedHeaders = "x-ca-key,x-ca-nonce,x-ca-timestamp"
	stringToSign := strings.Join([]string{
		http.MethodPost,
		"*/*",
		base64.StdEncoding.EncodeToString(contentMD5[:]),
		"application/json",
		date,
		"x-ca-key:" + appKey,
		"x-ca-nonce:" + nonce,
		"x-ca-timestamp:" + timestamp,
		req.URL.RequestURI(),
	}, "\n")
	hmacHash := hmac.New(sha256.New, []byte(appSecret))
	_, _ = hmacHash.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(hmacHash.Sum(nil))
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(contentMD5[:]))
	req.Header.Set("Date", date)
	req.Header.Set("X-Ca-Key", appKey)
	req.Header.Set("X-Ca-Nonce", nonce)
	req.Header.Set("X-Ca-Timestamp", timestamp)
	req.Header.Set("X-Ca-Signature-Headers", signedHeaders)
	req.Header.Set("X-Ca-Signature", signature)
}

type hikvisionPreviewResponse struct {
	Code any             `json:"code"`
	Msg  string          `json:"msg"`
	Data hikvisionStream `json:"data"`
}

type hikvisionStream struct {
	URL        string          `json:"url"`
	Protocol   string          `json:"protocol"`
	ExpireTime json.RawMessage `json:"expireTime"`
}

func decodeHikvisionPreviewResponse(body []byte) (model.VideoStream, error) {
	var response hikvisionPreviewResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return model.VideoStream{}, fmt.Errorf("decode Hikvision Artemis response: %w", err)
	}
	if code := fmt.Sprint(response.Code); code != "0" && code != "200" {
		return model.VideoStream{}, fmt.Errorf("Hikvision Artemis failed: code=%s msg=%s", code, strings.TrimSpace(response.Msg))
	}
	streamURL := strings.TrimSpace(response.Data.URL)
	if streamURL == "" {
		return model.VideoStream{}, errors.New("Hikvision Artemis did not return a preview URL")
	}
	if err := validateSourceURL(streamURL); err != nil {
		return model.VideoStream{}, fmt.Errorf("Hikvision Artemis returned an invalid preview URL: %w", err)
	}
	return model.VideoStream{
		URL:        streamURL,
		StreamType: sourceType(streamURL, response.Data.Protocol),
		ExpiresAt:  parseHikvisionTimestamp(response.Data.ExpireTime),
	}, nil
}

func parseHikvisionTimestamp(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil && number.String() != "" {
		if value, err := strconv.ParseInt(number.String(), 10, 64); err == nil {
			if value > 0 && value < 100000000000 {
				return value * 1000
			}
			return value
		}
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return 0
	}
	value = strings.TrimSpace(value)
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		if parsed > 0 && parsed < 100000000000 {
			return parsed * 1000
		}
		return parsed
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05.000-07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
