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

	"iot-platform/internal/ports"
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
	return w.IndexKnowledge(ctx, ports.KnowledgeIndexInput{TenantID: tenant, ProductID: product, DocumentID: id, ChunkID: id, Content: data})
}
func (w *Weaviate) IndexKnowledge(ctx context.Context, in ports.KnowledgeIndexInput) error {
	w.once.Do(func() { w.initErr = w.ensureClass(ctx) })
	if w.initErr != nil {
		return w.initErr
	}
	body, _ := json.Marshal(map[string]any{"class": "IotKnowledge", "id": in.ChunkID, "properties": map[string]any{"tenantId": in.TenantID, "productId": in.ProductID, "documentId": in.DocumentID, "category": in.Category, "tags": in.Tags, "content": string(in.Content)}})
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
	if resp.StatusCode/100 == 2 {
		defer resp.Body.Close()
		var existing struct {
			Properties []struct {
				Name string `json:"name"`
			} `json:"properties"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&existing); err != nil {
			return err
		}
		present := map[string]bool{}
		for _, property := range existing.Properties {
			present[property.Name] = true
		}
		for _, property := range knowledgeProperties() {
			name, _ := property["name"].(string)
			if present[name] {
				continue
			}
			body, _ := json.Marshal(property)
			create, createErr := http.NewRequestWithContext(ctx, http.MethodPost, w.url+"/v1/schema/IotKnowledge/properties", bytes.NewReader(body))
			if createErr != nil {
				return createErr
			}
			create.Header.Set("Content-Type", "application/json")
			created, createErr := w.http.Do(create)
			if createErr != nil {
				return createErr
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(created.Body, 4096))
			created.Body.Close()
			if created.StatusCode/100 != 2 {
				return fmt.Errorf("weaviate add property %s: %s", name, created.Status)
			}
		}
		return nil
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("weaviate schema check %s", resp.Status)
	}
	body, _ := json.Marshal(map[string]any{"class": "IotKnowledge", "vectorizer": "text2vec-ollama", "properties": knowledgeProperties()})
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
func knowledgeProperties() []map[string]any {
	return []map[string]any{
		{"name": "tenantId", "dataType": []string{"text"}, "tokenization": "field"},
		{"name": "productId", "dataType": []string{"text"}, "tokenization": "field"},
		{"name": "documentId", "dataType": []string{"text"}, "tokenization": "field"},
		{"name": "category", "dataType": []string{"text"}, "tokenization": "field"},
		{"name": "tags", "dataType": []string{"text"}, "tokenization": "field"},
		{"name": "content", "dataType": []string{"text"}},
	}
}
func (w *Weaviate) Search(ctx context.Context, tenant, q string, limit int) ([]string, error) {
	hits, err := w.SearchKnowledge(ctx, ports.KnowledgeSearchRequest{TenantID: tenant, Question: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Content)
	}
	return out, nil
}
func (w *Weaviate) SearchKnowledge(ctx context.Context, in ports.KnowledgeSearchRequest) ([]ports.KnowledgeHit, error) {
	w.once.Do(func() { w.initErr = w.ensureClass(ctx) })
	if w.initErr != nil {
		return nil, w.initErr
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 5
	}
	operands := []string{fmt.Sprintf(`{path:["tenantId"],operator:Equal,valueText:%q}`, in.TenantID)}
	if len(in.ProductIDs) > 0 {
		operands = append(operands, orTextFilter("productId", in.ProductIDs))
	}
	if len(in.Categories) > 0 {
		operands = append(operands, orTextFilter("category", in.Categories))
	}
	where := operands[0]
	if len(operands) > 1 {
		where = `{operator:And,operands:[` + strings.Join(operands, ",") + `]}`
	}
	// Fetch extra candidates because tag matching is applied after vector search.
	candidateLimit := limit
	if len(in.Tags) > 0 {
		candidateLimit = min(100, max(limit*5, 20))
	}
	query := fmt.Sprintf(`{Get{IotKnowledge(where:%s,nearText:{concepts:[%q]},limit:%d){documentId productId category tags content _additional{certainty}}}}`, where, in.Question, candidateLimit)
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
					DocumentID string   `json:"documentId"`
					ProductID  string   `json:"productId"`
					Category   string   `json:"category"`
					Tags       []string `json:"tags"`
					Content    string   `json:"content"`
					Additional struct {
						Certainty float64 `json:"certainty"`
					} `json:"_additional"`
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
	out := []ports.KnowledgeHit{}
	for _, v := range raw.Data.Get.Items {
		if !containsAll(v.Tags, in.Tags) || v.Additional.Certainty < in.MinScore {
			continue
		}
		out = append(out, ports.KnowledgeHit{DocumentID: v.DocumentID, ProductID: v.ProductID, Category: v.Category, Tags: v.Tags, Content: v.Content, Score: v.Additional.Certainty})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func orTextFilter(path string, values []string) string {
	operands := make([]string, 0, len(values))
	for _, value := range values {
		operands = append(operands, fmt.Sprintf(`{path:[%q],operator:Equal,valueText:%q}`, path, value))
	}
	if len(operands) == 1 {
		return operands[0]
	}
	return `{operator:Or,operands:[` + strings.Join(operands, ",") + `]}`
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
