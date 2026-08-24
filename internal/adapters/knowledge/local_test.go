package knowledge

import (
	"context"
	"testing"

	"iot-platform/internal/ports"
)

func TestLocalKnowledgeAppliesWorkflowMetadataFilters(t *testing.T) {
	index := NewLocal()
	for _, input := range []ports.KnowledgeIndexInput{
		{TenantID: "tenant-a", ProductID: "smoke", Category: "alarm-sop", Tags: []string{"certified", "fire"}, DocumentID: "doc-1", ChunkID: "chunk-1", Content: []byte("烟雾 告警 现场 复核 处置")},
		{TenantID: "tenant-a", ProductID: "camera", Category: "manual", Tags: []string{"video"}, DocumentID: "doc-2", ChunkID: "chunk-2", Content: []byte("烟雾 告警 摄像头 联动")},
		{TenantID: "tenant-b", ProductID: "smoke", Category: "alarm-sop", Tags: []string{"certified", "fire"}, DocumentID: "doc-3", ChunkID: "chunk-3", Content: []byte("烟雾 告警 其他租户")},
	} {
		if err := index.IndexKnowledge(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := index.SearchKnowledge(context.Background(), ports.KnowledgeSearchRequest{TenantID: "tenant-a", Question: "烟雾 告警", ProductIDs: []string{"smoke"}, Categories: []string{"alarm-sop"}, Tags: []string{"certified"}, Limit: 5, MinScore: .5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].DocumentID != "doc-1" || hits[0].ProductID != "smoke" || hits[0].Score < .5 {
		t.Fatalf("unexpected filtered hits: %#v", hits)
	}
}

func TestLocalKnowledgeSupportsSingleCharacterChineseQueries(t *testing.T) {
	index := NewLocal()
	if err := index.IndexKnowledge(context.Background(), ports.KnowledgeIndexInput{TenantID: "tenant-a", DocumentID: "doc-water", ChunkID: "chunk-water", Content: []byte("水压异常处置")}); err != nil {
		t.Fatal(err)
	}
	hits, err := index.SearchKnowledge(context.Background(), ports.KnowledgeSearchRequest{TenantID: "tenant-a", Question: "水", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].DocumentID != "doc-water" {
		t.Fatalf("single-character query returned %#v", hits)
	}
}
