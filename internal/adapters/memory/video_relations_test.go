package memory

import (
	"context"
	"testing"

	"iot-platform/internal/model"
)

func TestVideoCameraRelationsSupportReverseLookup(t *testing.T) {
	repo := NewRepository()
	camera := model.VideoCameraMapping{TenantID: "tenant-001", CameraID: "camera-001", CameraName: "一号摄像头", RelatedDeviceIDs: []string{"device-001", "device-002"}, RelatedFloorIDs: []string{"floor-01"}, RelatedRoomIDs: []string{"room-101", "room-102"}}
	if err := repo.SaveVideoCameraMapping(context.Background(), camera); err != nil {
		t.Fatal(err)
	}
	relations, err := repo.ListVideoCameraRelationsByTarget(context.Background(), "tenant-001", "device", "device-002")
	if err != nil {
		t.Fatal(err)
	}
	if len(relations) != 1 || relations[0].CameraID != "camera-001" {
		t.Fatalf("reverse device lookup = %#v", relations)
	}
	roomRelations, err := repo.ListVideoCameraRelations(context.Background(), "tenant-001", "camera-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(roomRelations) != 5 {
		t.Fatalf("camera relations = %#v", roomRelations)
	}
}
