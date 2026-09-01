package storage

import (
	"path/filepath"
	"testing"
)

func TestSQLiteQueueOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_queue.db")

	queue, err := NewSQLiteQueue(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteQueue failed: %v", err)
	}
	defer queue.Close()

	// Initial count should be 0
	count, err := queue.Count()
	if err != nil || count != 0 {
		t.Fatalf("Expected initial count 0, got %d (err: %v)", count, err)
	}

	// Enqueue 2 test payloads
	payload1 := `{"metadata":{"tenant_id":"t1","device_uuid":"d1"}}`
	payload2 := `{"metadata":{"tenant_id":"t1","device_uuid":"d2"}}`

	if err := queue.Enqueue(payload1); err != nil {
		t.Fatalf("Enqueue payload1 failed: %v", err)
	}
	if err := queue.Enqueue(payload2); err != nil {
		t.Fatalf("Enqueue payload2 failed: %v", err)
	}

	count, err = queue.Count()
	if err != nil || count != 2 {
		t.Fatalf("Expected count 2 after enqueue, got %d", count)
	}

	// Dequeue all
	items, err := queue.DequeueAll()
	if err != nil || len(items) != 2 {
		t.Fatalf("Expected 2 items from DequeueAll, got %d (err: %v)", len(items), err)
	}

	if items[0].Payload != payload1 {
		t.Errorf("Expected item 0 payload '%s', got '%s'", payload1, items[0].Payload)
	}

	// Delete item 0
	if err := queue.Delete([]int64{items[0].ID}); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	count, err = queue.Count()
	if err != nil || count != 1 {
		t.Fatalf("Expected count 1 after delete, got %d", count)
	}
}
