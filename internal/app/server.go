package app

import (
	"encoding/gob"
	"fmt"
	"focus/internal/domain"
	"focus/internal/effects"
	"focus/internal/protocol"
	"focus/internal/storage"
	"net"
	"strings"
	"time"
)

type Server struct {
	runtime        *Runtime
	actions        effects.Actions
	reloadConfig   func() error
	statusProvider func() string
}

func NewServer(rt *Runtime, actions effects.Actions, reloadConfig func() error) *Server {
	if actions == nil {
		actions = effects.RealActions{}
	}
	return &Server{runtime: rt, actions: actions, reloadConfig: reloadConfig}
}

func (s *Server) SetStatusProvider(fn func() string) {
	s.statusProvider = fn
}

func (s *Server) HandleConnection(conn net.Conn) {
	defer conn.Close()
	var req protocol.Request
	if err := gob.NewDecoder(conn).Decode(&req); err != nil {
		fmt.Printf("Decode error: %v\n", err)
		return
	}
	res := s.handleRequest(req)
	if err := gob.NewEncoder(conn).Encode(res); err != nil {
		fmt.Printf("Encode error: %v\n", err)
		return
	}
}

func (s *Server) handleRequest(req protocol.Request) protocol.Response {
	switch req.Command {
	case "start":
		if req.Start == nil {
			return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: "missing start payload"}}
		}
		return s.handleStart(*req.Start)
	case "status":
		return s.handleStatus()
	case "cancel":
		return s.handleCancel()
	case "history":
		return s.handleHistory(req.HistoryAll)
	case "reload":
		return s.handleReload()
	default:
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: fmt.Sprintf("Unknown command: %s", req.Command)}}
	}
}

func (s *Server) handleReload() protocol.Response {
	if s.reloadConfig == nil {
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: "reload is not available"}}
	}
	if err := s.reloadConfig(); err != nil {
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: fmt.Sprintf("reload failed: %v", err)}}
	}
	return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: "Config reloaded."}}
}

func (s *Server) handleStart(req protocol.StartRequest) protocol.Response {
	duration := req.Duration
	if req.Preset != "" {
		resolvedDuration, err := storage.ResolveTaskPresetDuration(req.Preset)
		if err != nil {
			return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: err.Error()}}
		}
		duration = resolvedDuration
	}
	if duration <= 0 {
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: "missing task duration; provide a preset (short|medium|long|deep)"}}
	}
	task, err := s.runtime.StartTask(req.Title, duration, req.NoBreak)
	if err != nil {
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: err.Error()}}
	}
	s.actions.Notify("Task Started", fmt.Sprintf("Started task: %s for %s", task.Title, task.Duration))
	return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: fmt.Sprintf("Started task: %s for %s", task.Title, task.Duration)}}
}

func (s *Server) handleStatus() protocol.Response {
	message := s.runtime.Status()
	if s.statusProvider != nil {
		message = s.statusProvider()
	}
	return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: message}}
}

func (s *Server) handleCancel() protocol.Response {
	task, err := s.runtime.CancelCurrentTask()
	if err != nil {
		return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: fmt.Sprintf("Failed to cancel task: %v", err)}}
	}
	s.actions.Notify("Task Cancelled", fmt.Sprintf("Cancelled the task: %s", task.Title))
	return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: fmt.Sprintf("Cancelled the task: %s", task.Title)}}
}

func (s *Server) handleHistory(all bool) protocol.Response {
	if all {
		entries, loadErr := storage.LoadAllHistory()
		if loadErr != nil {
			return protocol.Response{Type: "error", Error: &protocol.ErrorResponse{Message: fmt.Sprintf("Failed to load history: %v", loadErr)}}
		}
		lines := make([]string, 0, len(entries))
		for _, entry := range entries {
			lines = append(lines, formatHistoryLine(entry.Task))
		}
		if len(lines) == 0 {
			return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: "No task history"}}
		}
		return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: strings.Join(lines, "\n")}}
	}
	history := s.runtime.History()
	if len(history) == 0 {
		return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: "No task history"}}
	}
	lines := make([]string, 0, len(history))
	for _, task := range history {
		lines = append(lines, formatHistoryLine(task))
	}
	return protocol.Response{Type: "success", Success: &protocol.SuccessResponse{Message: strings.Join(lines, "\n")}}
}

func formatHistoryLine(task domain.Task) string {
	return fmt.Sprintf("[%d] %s | %s | completed | started %s", task.ID, task.Title, task.Duration.Round(time.Second), task.StartTime.Format(time.RFC3339))
}
