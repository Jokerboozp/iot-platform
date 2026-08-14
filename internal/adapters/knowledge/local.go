package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
)

type Local struct {
	mu   sync.RWMutex
	docs map[string][]document
}
type document struct{ id, product, text string }

func NewLocal() *Local { return &Local{docs: map[string][]document{}} }
func (k *Local) Index(_ context.Context, tenant, product, id string, data []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.docs[tenant] = append(k.docs[tenant], document{id, product, string(data)})
	return nil
}
func (k *Local) Search(_ context.Context, tenant, q string, limit int) ([]string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if limit <= 0 {
		limit = 5
	}
	terms := tokens(q)
	type hit struct {
		score int
		text  string
	}
	hits := []hit{}
	for _, d := range k.docs[tenant] {
		score := 0
		lower := strings.ToLower(d.text)
		for _, t := range terms {
			score += strings.Count(lower, t)
		}
		if score > 0 {
			snippet := d.text
			if len([]rune(snippet)) > 800 {
				snippet = string([]rune(snippet)[:800])
			}
			hits = append(hits, hit{score, snippet})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := []string{}
	for i, h := range hits {
		if i >= limit {
			break
		}
		out = append(out, h.text)
	}
	return out, nil
}
func (k *Local) Health(context.Context) error { return nil }
func tokens(s string) []string {
	f := func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",，。；;:：/\\()（）", r) }
	out := []string{}
	for _, v := range strings.FieldsFunc(strings.ToLower(s), f) {
		if len([]rune(v)) > 1 {
			out = append(out, v)
		}
	}
	return out
}

var _ = fmt.Sprint
