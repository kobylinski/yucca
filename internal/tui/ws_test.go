package tui

import (
	"encoding/json"
	"testing"
)

func TestWSEventMsgParsing(t *testing.T) {
	raw := `{"type":"request_created","project":"my-project"}`
	var event wsEventMsg
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event.Type != "request_created" {
		t.Errorf("expected type request_created, got %s", event.Type)
	}
	if event.Project != "my-project" {
		t.Errorf("expected project my-project, got %s", event.Project)
	}

	// Without project field
	raw2 := `{"type":"sessions_changed"}`
	var event2 wsEventMsg
	if err := json.Unmarshal([]byte(raw2), &event2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event2.Type != "sessions_changed" {
		t.Errorf("expected type sessions_changed, got %s", event2.Type)
	}
	if event2.Project != "" {
		t.Errorf("expected empty project, got %s", event2.Project)
	}
}

func TestWSEventMsgTypes(t *testing.T) {
	types := []struct {
		eventType string
		project   string
	}{
		{"request_created", "proj-a"},
		{"request_resolved", "proj-b"},
		{"credentials_changed", "proj-c"},
		{"sessions_changed", ""},
	}

	for _, tt := range types {
		t.Run(tt.eventType, func(t *testing.T) {
			original := wsEventMsg{Type: tt.eventType, Project: tt.project}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded wsEventMsg
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if decoded.Type != original.Type {
				t.Errorf("type mismatch: got %s, want %s", decoded.Type, original.Type)
			}
			if decoded.Project != original.Project {
				t.Errorf("project mismatch: got %s, want %s", decoded.Project, original.Project)
			}
		})
	}
}
