package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Weaviate uses a text2vec-enabled IotKnowledge class. Compose initializes this class.
type Weaviate struct {
	url     string
	http    *http.Client
	once    sync.Once
	initErr error
}

func NewWeaviate(url string) *Weaviate {
	return &Weaviate{url: strings.TrimRight(url, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}
func (w *Weaviate) Index(ctx context.Context, tenant, product, id string, data []byte) error {
	w.once.Do(func() { w.initErr = w.ensureClass(ctx) })
	if w.initErr != nil {
		return w.initErr
	}
	body, _ := json.Marshal(map[string]any{"class": "IotKnowledge", "id": id, "properties": map[string]any{"tenantId": tenant, "productId": product, "content": string(data)}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/v1/objects", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != 422 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("weaviate index %s: %s", resp.Status, string(b))
	}
	return nil
}

func (w *Weaviate) ensureClass(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, w.url+"/v1/schema/IotKnowledge", nil)
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("weaviate schema check %s", resp.Status)
	}
	body, _ := json.Marshal(map[string]any{"class": "IotKnowledge", "vectorizer": "text2vec-ollama", "properties": []map[string]any{{"name": "tenantId", "dataType": []string{"text"}, "tokenization": "field"}, {"name": "productId", "dataType": []string{"text"}, "tokenization": "field"}, {"name": "content", "dataType": []string{"text"}}}})
	create, _ := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/v1/schema", bytes.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	created, err := w.http.Do(create)
	if err != nil {
		return err
	}
	defer created.Body.Close()
	if created.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(created.Body, 4096))
		return fmt.Errorf("weaviate create class %s: %s", created.Status, string(b))
	}
	return nil
}
func (w *Weaviate) Search(ctx context.Context, tenant, q string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5
	}
	query := fmt.Sprintf(`{Get{IotKnowledge(where:{path:["tenantId"],operator:Equal,valueText:%q},nearText:{concepts:[%q]},limit:%d){content}}}`, tenant, q, limit)
	body, _ := json.Marshal(map[string]string{"query": query})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/v1/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("weaviate search %s", resp.Status)
	}
	var raw struct {
		Data struct {
			Get struct {
				Items []struct {
					Content string `json:"content"`
				} `json:"IotKnowledge"`
			} `json:"Get"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql: %v", raw.Errors)
	}
	out := []string{}
	for _, v := range raw.Data.Get.Items {
		out = append(out, v.Content)
	}
	return out, nil
}
func (w *Weaviate) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, w.url+"/v1/.well-known/ready", nil)
	resp, err := w.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("weaviate %s", resp.Status)
	}
	return nil
}
