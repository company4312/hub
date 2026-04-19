package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/company4312/copilot-telegram-bot/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := New(s, nil, ":0")
	return srv, s
}

func do(t *testing.T, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rr, req)
	return rr
}

func TestGetAgents_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/api/agents", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestGetAgents_WithRegistered(t *testing.T) {
	srv, s := newTestServer(t)
	_ = s.RegisterAgent(store.AgentConfig{Name: "atlas", Title: "Atlas", SystemPrompt: "p", Model: "m"})
	rr := do(t, srv, http.MethodGet, "/api/agents", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var agents []store.AgentConfig
	_ = json.Unmarshal(rr.Body.Bytes(), &agents)
	if len(agents) != 1 || agents[0].Name != "atlas" {
		t.Errorf("unexpected: %+v", agents)
	}
}

func TestCreateMemory(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{"agent_name": "atlas", "category": "context", "content": "test memory"}
	rr := do(t, srv, http.MethodPost, "/api/memories", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreateMemory_MissingFields(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodPost, "/api/memories", map[string]string{"agent_name": "a"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestListThreads_Empty(t *testing.T) {
	srv, _ := newTestServer(t)
	rr := do(t, srv, http.MethodGet, "/api/threads", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListThreads_WithData(t *testing.T) {
	srv, s := newTestServer(t)
	_ = s.CreateThread(store.Thread{ID: "t1", ChatID: 1, Title: "Test", Status: "active"})
	_, _ = s.AddMessage(store.Message{ThreadID: "t1", FromName: "user", Content: "hello", MessageType: "user_message"})

	rr := do(t, srv, http.MethodGet, "/api/threads", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var threads []map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &threads)
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0]["last_message"] == nil {
		t.Error("expected last_message preview")
	}
}

func TestGetThreadMessages(t *testing.T) {
	srv, s := newTestServer(t)
	_ = s.CreateThread(store.Thread{ID: "t1", ChatID: 1, Title: "Test", Status: "active"})
	_, _ = s.AddMessage(store.Message{ThreadID: "t1", FromName: "user", Content: "hi", MessageType: "user_message"})
	_, _ = s.AddMessage(store.Message{ThreadID: "t1", FromName: "cto", Content: "hello", MessageType: "agent_message"})

	rr := do(t, srv, http.MethodGet, "/api/threads/t1/messages", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var msgs []store.Message
	_ = json.Unmarshal(rr.Body.Bytes(), &msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGetMessageEvents(t *testing.T) {
	srv, s := newTestServer(t)
	_ = s.CreateThread(store.Thread{ID: "t1", ChatID: 1, Title: "T", Status: "active"})
	msgID, _ := s.AddMessage(store.Message{ThreadID: "t1", FromName: "cto", Content: "done", MessageType: "agent_message", CopilotSessionID: "sess-1"})
	_ = s.AddSessionEvent(store.SessionEvent{CopilotSessionID: "sess-1", ThreadID: "t1", AgentName: "cto", EventType: "tool_call", Content: "git"})

	rr := do(t, srv, http.MethodGet, fmt.Sprintf("/api/messages/%d/events", msgID), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var events []store.SessionEvent
	_ = json.Unmarshal(rr.Body.Bytes(), &events)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestCreateProject(t *testing.T) {
	srv, _ := newTestServer(t)
	body := map[string]string{"id": "proj-1", "name": "My Project", "created_by": "cto"}
	rr := do(t, srv, http.MethodPost, "/api/projects", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestPatchTask(t *testing.T) {
	srv, s := newTestServer(t)
	_ = s.CreateProject(store.Project{ID: "p", Name: "P", Status: "active", CreatedBy: "x"})
	_ = s.CreateTask(store.Task{ID: "t1", ProjectID: "p", Title: "Old", Status: "backlog", CreatedBy: "x", Priority: 3})

	rr := do(t, srv, http.MethodPatch, "/api/tasks/t1", map[string]any{"title": "New Title"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d, body=%s", rr.Code, rr.Body.String())
	}
	task, _ := s.GetTask("t1")
	if task.Title != "New Title" {
		t.Errorf("expected 'New Title', got %q", task.Title)
	}
}
