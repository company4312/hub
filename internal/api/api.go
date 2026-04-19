package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/company4312/copilot-telegram-bot/internal/store"
	"github.com/company4312/copilot-telegram-bot/web"
)

// AgentPool is the interface the API server uses to interact with agents.
type AgentPool interface {
	InvokeAgent(ctx context.Context, threadID, fromAgent, toAgent, text string, parentMsgID *int64) (string, error)
	DelegateTask(ctx context.Context, fromAgent, toAgent string, chatID int64, projectID, taskID, title, instructions string, priority int, threadID string) error
}

// Server is the HTTP API server for the Company4312 dashboard.
type Server struct {
	store      *store.Store
	pool       AgentPool
	addr       string
	httpServer *http.Server

	mu      sync.Mutex
	clients map[chan store.Message]struct{}
}

// New creates a new API server.
func New(s *store.Store, pool AgentPool, addr string) *Server {
	srv := &Server{
		store:   s,
		pool:    pool,
		addr:    addr,
		clients: make(map[chan store.Message]struct{}),
	}

	mux := http.NewServeMux()

	// Agent endpoints.
	mux.HandleFunc("/api/agents/", srv.handleAgentByName)
	mux.HandleFunc("/api/agents", srv.handleAgents)

	// Thread endpoints.
	mux.HandleFunc("/api/threads/stream", srv.handleThreadStream)
	mux.HandleFunc("/api/threads/", srv.handleThreadByID)
	mux.HandleFunc("/api/threads", srv.handleThreads)

	// Message events endpoint.
	mux.HandleFunc("/api/messages/", srv.handleMessageByID)

	// Memory endpoints.
	mux.HandleFunc("/api/memories/", srv.handleMemoryByID)
	mux.HandleFunc("/api/memories", srv.handleMemories)

	// Project endpoints.
	mux.HandleFunc("/api/projects/", srv.handleProjectByID)
	mux.HandleFunc("/api/projects", srv.handleProjects)

	// Task endpoints.
	mux.HandleFunc("/api/tasks/delegate", srv.handleDelegateTask)
	mux.HandleFunc("/api/tasks/", srv.handleTaskByID)
	mux.HandleFunc("/api/tasks", srv.handleTasks)

	// Serve embedded frontend.
	distFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("embed frontend: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

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

// Broadcast sends a message to all connected SSE clients.
func (srv *Server) Broadcast(msg store.Message) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for ch := range srv.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// --- Agent endpoints ---

func (srv *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		srv.handleListAgents(w, r)
	case http.MethodPost:
		srv.handleCreateAgent(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleListAgents(w http.ResponseWriter, _ *http.Request) {
	agents, err := srv.store.ListAgents()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agents)
}

func (srv *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var body store.AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Title == "" || body.SystemPrompt == "" || body.Model == "" {
		http.Error(w, "name, title, system_prompt, and model are required", http.StatusBadRequest)
		return
	}
	if err := srv.store.RegisterAgent(body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (srv *Server) handleAgentByName(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Path[len("/api/agents/"):]
	if name == "" {
		http.Error(w, "agent name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		agent, err := srv.store.GetAgent(name)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if agent == nil {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agent)
	case http.MethodPut:
		var body store.AgentConfig
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		body.Name = name
		if err := srv.store.RegisterAgent(body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Thread endpoints ---

func (srv *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	filter := store.ThreadFilter{
		Status: q.Get("status"),
	}
	if v := q.Get("chat_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ChatID = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	threads, err := srv.store.ListThreads(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if threads == nil {
		threads = []store.Thread{}
	}

	// Attach last message preview to each thread.
	type threadWithPreview struct {
		store.Thread
		LastMessage *store.Message `json:"last_message,omitempty"`
	}
	out := make([]threadWithPreview, len(threads))
	for i, t := range threads {
		out[i].Thread = t
		if msg, err := srv.store.GetLastMessage(t.ID); err == nil {
			out[i].LastMessage = msg
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (srv *Server) handleThreadByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/threads/"):]
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "thread id required", http.StatusBadRequest)
		return
	}

	threadID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "messages":
			srv.handleThreadMessages(w, r, threadID)
			return
		case "invoke":
			srv.handleThreadInvoke(w, r, threadID)
			return
		}
	}

	if len(parts) > 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	t, err := srv.store.GetThread(threadID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "thread not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(t)
}

func (srv *Server) handleThreadMessages(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	msgs, err := srv.store.ListMessages(threadID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []store.Message{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msgs)
}

func (srv *Server) handleThreadInvoke(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if srv.pool == nil {
		http.Error(w, "agent pool not available", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		From            string `json:"from"`
		To              string `json:"to"`
		Message         string `json:"message"`
		ParentMessageID *int64 `json:"parent_message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" || body.Message == "" {
		http.Error(w, "from, to, and message are required", http.StatusBadRequest)
		return
	}

	// Verify thread exists.
	t, err := srv.store.GetThread(threadID)
	if err != nil || t == nil {
		http.Error(w, "thread not found", http.StatusNotFound)
		return
	}

	response, err := srv.pool.InvokeAgent(r.Context(), threadID, body.From, body.To, body.Message, body.ParentMessageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("invoke agent: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"response": response,
		"agent":    body.To,
	})
}

func (srv *Server) handleThreadStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := make(chan store.Message, 64)
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
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// --- Message endpoints ---

func (srv *Server) handleMessageByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/messages/"):]
	parts := splitPath(path)
	if len(parts) < 2 || parts[1] != "events" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	msgID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid message ID", http.StatusBadRequest)
		return
	}

	events, err := srv.store.ListSessionEventsByMessage(msgID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []store.SessionEvent{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

// --- Memory endpoints ---

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
	if !store.ValidMemoryCategories[body.Category] {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	if len(body.Content) > store.MaxMemoryContent {
		http.Error(w, "content too long", http.StatusBadRequest)
		return
	}
	if len(body.AgentName) > 100 {
		http.Error(w, "agent_name too long", http.StatusBadRequest)
		return
	}

	id, err := srv.store.SaveMemory(store.Memory{
		AgentName: body.AgentName,
		Category:  body.Category,
		Content:   body.Content,
		Source:    body.Source,
	})
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

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
		var body struct {
			AgentName string `json:"agent_name"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Content == "" || body.AgentName == "" {
			http.Error(w, "agent_name and content are required", http.StatusBadRequest)
			return
		}
		if len(body.Content) > store.MaxMemoryContent {
			http.Error(w, "content too long", http.StatusBadRequest)
			return
		}
		if err := srv.store.UpdateMemory(id, body.AgentName, body.Content); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		var body struct {
			AgentName string `json:"agent_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentName == "" {
			http.Error(w, "agent_name is required", http.StatusBadRequest)
			return
		}
		if err := srv.store.DeleteMemory(id, body.AgentName); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Project endpoints ---

func (srv *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := srv.store.ListProjects()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if projects == nil {
			projects = []store.Project{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(projects)
	case http.MethodPost:
		srv.handleCreateProject(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		CreatedBy   string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.Name == "" || body.CreatedBy == "" {
		http.Error(w, "id, name, and created_by are required", http.StatusBadRequest)
		return
	}
	if len(body.Name) > store.MaxTitleLength {
		http.Error(w, "name too long", http.StatusBadRequest)
		return
	}
	if err := srv.store.CreateProject(store.Project{
		ID: body.ID, Name: body.Name, Description: body.Description,
		Status: "active", CreatedBy: body.CreatedBy,
	}); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": body.ID})
}

func (srv *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/projects/"):]
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "project id required", http.StatusBadRequest)
		return
	}
	projectID := parts[0]

	if len(parts) == 2 && parts[1] == "tasks" {
		srv.handleProjectTasks(w, r, projectID)
		return
	}

	if len(parts) > 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := srv.store.GetProject(projectID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	case http.MethodPatch:
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}
		if !store.ValidProjectStatuses[body.Status] {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
		if err := srv.store.UpdateProjectStatus(projectID, body.Status); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleProjectTasks(w http.ResponseWriter, r *http.Request, projectID string) {
	switch r.Method {
	case http.MethodGet:
		tasks, err := srv.store.ListTasks(store.TaskFilter{ProjectID: projectID})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tasks == nil {
			tasks = []store.Task{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tasks)
	case http.MethodPost:
		var body struct {
			ID          string  `json:"id"`
			Title       string  `json:"title"`
			Description string  `json:"description"`
			AssignedTo  *string `json:"assigned_to"`
			CreatedBy   string  `json:"created_by"`
			Priority    int     `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.ID == "" || body.Title == "" || body.CreatedBy == "" {
			http.Error(w, "id, title, and created_by are required", http.StatusBadRequest)
			return
		}
		if len(body.Title) > store.MaxTitleLength {
			http.Error(w, "title too long", http.StatusBadRequest)
			return
		}
		if body.Priority < 1 || body.Priority > 4 {
			body.Priority = 3
		}
		p, err := srv.store.GetProject(projectID)
		if err != nil || p == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		if err := srv.store.CreateTask(store.Task{
			ID: body.ID, ProjectID: projectID, Title: body.Title,
			Description: body.Description, Status: "backlog",
			AssignedTo: body.AssignedTo, CreatedBy: body.CreatedBy,
			Priority: body.Priority,
		}); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": body.ID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Task endpoints ---

func (srv *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	filter := store.TaskFilter{
		ProjectID:  q.Get("project_id"),
		Status:     q.Get("status"),
		AssignedTo: q.Get("assigned_to"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	tasks, err := srv.store.ListTasks(filter)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		tasks = []store.Task{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tasks)
}

func (srv *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/tasks/"):]
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}
	taskID := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "status":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			t, err := srv.store.GetTask(taskID)
			if err != nil || t == nil {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			assignedTo := ""
			if t.AssignedTo != nil {
				assignedTo = *t.AssignedTo
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"task_id": t.ID, "status": t.Status,
				"title": t.Title, "assigned_to": assignedTo,
			})
			return
		case "dependencies":
			srv.handleTaskDependencies(w, r, taskID)
			return
		case "comments":
			srv.handleTaskComments(w, r, taskID)
			return
		}
	}

	if len(parts) > 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, err := srv.store.GetTask(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if t == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(t)
	case http.MethodPatch:
		existing, err := srv.store.GetTask(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		var body struct {
			Title       *string `json:"title"`
			Description *string `json:"description"`
			Status      *string `json:"status"`
			AssignedTo  *string `json:"assigned_to"`
			Priority    *int    `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Title != nil {
			if len(*body.Title) > store.MaxTitleLength {
				http.Error(w, "title too long", http.StatusBadRequest)
				return
			}
			existing.Title = *body.Title
		}
		if body.Description != nil {
			existing.Description = *body.Description
		}
		if body.Status != nil {
			if !store.ValidTaskStatuses[*body.Status] {
				http.Error(w, "invalid status", http.StatusBadRequest)
				return
			}
			existing.Status = *body.Status
		}
		if body.AssignedTo != nil {
			existing.AssignedTo = body.AssignedTo
		}
		if body.Priority != nil {
			if *body.Priority < 1 || *body.Priority > 4 {
				http.Error(w, "priority must be 1-4", http.StatusBadRequest)
				return
			}
			existing.Priority = *body.Priority
		}
		if err := srv.store.UpdateTask(*existing); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleTaskDependencies(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		blocking, err := srv.store.GetBlockingTasks(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if blocking == nil {
			blocking = []store.Task{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(blocking)
	case http.MethodPost:
		var body struct {
			DependsOn string `json:"depends_on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.DependsOn == "" || body.DependsOn == taskID {
			http.Error(w, "invalid dependency", http.StatusBadRequest)
			return
		}
		if err := srv.store.AddTaskDependency(taskID, body.DependsOn); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleTaskComments(w http.ResponseWriter, r *http.Request, taskID string) {
	switch r.Method {
	case http.MethodGet:
		comments, err := srv.store.GetTaskComments(taskID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if comments == nil {
			comments = []store.TaskComment{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comments)
	case http.MethodPost:
		var body struct {
			AgentName string `json:"agent_name"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.AgentName == "" || body.Content == "" {
			http.Error(w, "agent_name and content are required", http.StatusBadRequest)
			return
		}
		if len(body.Content) > store.MaxCommentLength {
			http.Error(w, "comment too long", http.StatusBadRequest)
			return
		}
		t, err := srv.store.GetTask(taskID)
		if err != nil || t == nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		id, err := srv.store.AddTaskComment(store.TaskComment{
			TaskID: taskID, AgentName: body.AgentName, Content: body.Content,
		})
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (srv *Server) handleDelegateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if srv.pool == nil {
		http.Error(w, "agent pool not available", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		From         string `json:"from"`
		To           string `json:"to"`
		ProjectID    string `json:"project_id"`
		Title        string `json:"title"`
		Instructions string `json:"instructions"`
		Priority     int    `json:"priority"`
		ChatID       int64  `json:"chat_id"`
		ThreadID     string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" || body.Title == "" || body.Instructions == "" {
		http.Error(w, "from, to, title, and instructions are required", http.StatusBadRequest)
		return
	}
	if body.Priority < 1 || body.Priority > 4 {
		body.Priority = 2
	}

	taskID := fmt.Sprintf("task-%s-%d", body.To, time.Now().UnixMilli())

	if err := srv.pool.DelegateTask(r.Context(), body.From, body.To, body.ChatID,
		body.ProjectID, taskID, body.Title, body.Instructions, body.Priority, body.ThreadID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"task_id": taskID,
		"status":  "delegated",
	})
}

func splitPath(path string) []string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
