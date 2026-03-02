package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/company4312/copilot-telegram-bot/internal/store"
)

// Server is the HTTP API server for the Company4312 dashboard.
type Server struct {
	store      *store.Store
	addr       string
	httpServer *http.Server

	mu      sync.Mutex
	clients map[chan store.ActivityEntry]struct{}
}

// New creates a new API server.
func New(s *store.Store, addr string) *Server {
	srv := &Server{
		store:   s,
		addr:    addr,
		clients: make(map[chan store.ActivityEntry]struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents", srv.handleAgents)
	mux.HandleFunc("/api/activity/stream", srv.handleActivityStream)
	mux.HandleFunc("/api/activity", srv.handleActivity)
	mux.HandleFunc("/api/memories/", srv.handleMemoryByID)
	mux.HandleFunc("/api/memories", srv.handleMemories)
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))

	srv.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return srv
}

// Start begins listening for HTTP requests.
func (srv *Server) Start() error {
	ln, err := net.Listen("tcp", srv.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.addr, err)
	}
	log.Printf("Dashboard API listening on %s", srv.addr)
	go func() {
		if err := srv.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (srv *Server) Stop(ctx context.Context) error {
	return srv.httpServer.Shutdown(ctx)
}

// Broadcast sends an activity entry to all connected SSE clients.
func (srv *Server) Broadcast(entry store.ActivityEntry) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for ch := range srv.clients {
		select {
		case ch <- entry:
		default:
			// Drop if client is slow.
		}
	}
}

func (srv *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents, err := srv.store.ListAgents()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("list agents: %v", err)
		return
	}

	type agentInfo struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	out := make([]agentInfo, len(agents))
	for i, a := range agents {
		out[i] = agentInfo{Name: a.Name, Title: a.Title}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (srv *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	filter := store.ActivityFilter{
		AgentName: q.Get("agent"),
		EventType: q.Get("type"),
		Since:     q.Get("since"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	entries, err := srv.store.GetActivities(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get activities: %v", err)
		return
	}
	if entries == nil {
		entries = []store.ActivityEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

func (srv *Server) handleActivityStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := make(chan store.ActivityEntry, 64)
	srv.mu.Lock()
	srv.clients[ch] = struct{}{}
	srv.mu.Unlock()

	defer func() {
		srv.mu.Lock()
		delete(srv.clients, ch)
		srv.mu.Unlock()
		close(ch)
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-ch:
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (srv *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		srv.handleListMemories(w, r)
	case http.MethodPost:
		srv.handleCreateMemory(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.MemoryFilter{
		AgentName: q.Get("agent"),
		Category:  q.Get("category"),
		Search:    q.Get("search"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	memories, err := srv.store.GetMemories(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("get memories: %v", err)
		return
	}
	if memories == nil {
		memories = []store.Memory{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memories)
}

func (srv *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentName string `json:"agent_name"`
		Category  string `json:"category"`
		Content   string `json:"content"`
		Source    string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.AgentName == "" || body.Category == "" || body.Content == "" {
		http.Error(w, "agent_name, category, and content are required", http.StatusBadRequest)
		return
	}

	id, err := srv.store.SaveMemory(store.Memory{
		AgentName: body.AgentName,
		Category:  body.Category,
		Content:   body.Content,
		Source:    body.Source,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("save memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := store.ActivityEntry{
		Timestamp: now,
		AgentName: body.AgentName,
		EventType: "memory_created",
		Content:   body.Content,
		Metadata:  fmt.Sprintf(`{"category":"%s","source":"%s","memory_id":%d}`, body.Category, body.Source, id),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

func (srv *Server) handleMemoryByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/memories/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid memory ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		srv.handleUpdateMemory(w, r, id)
	case http.MethodDelete:
		srv.handleDeleteMemory(w, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request, id int64) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	if err := srv.store.UpdateMemory(id, body.Content); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("update memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := store.ActivityEntry{
		Timestamp: now,
		EventType: "memory_updated",
		Content:   body.Content,
		Metadata:  fmt.Sprintf(`{"memory_id":%d}`, id),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.WriteHeader(http.StatusNoContent)
}

func (srv *Server) handleDeleteMemory(w http.ResponseWriter, id int64) {
	if err := srv.store.DeleteMemory(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("delete memory: %v", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry := store.ActivityEntry{
		Timestamp: now,
		EventType: "memory_deleted",
		Content:   fmt.Sprintf("memory %d deleted", id),
		Metadata:  fmt.Sprintf(`{"memory_id":%d}`, id),
	}
	if err := srv.store.LogActivity(entry); err != nil {
		log.Printf("log activity: %v", err)
	}
	srv.Broadcast(entry)

	w.WriteHeader(http.StatusNoContent)
}
