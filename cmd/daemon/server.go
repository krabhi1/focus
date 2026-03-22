package main

import (
	"encoding/gob"
	"fmt"
	"focus/internal/core"
	"focus/internal/protocol"
	"focus/internal/state"
	"focus/internal/sys"
	"net"
	"strings"
	"time"
)

type Server struct {
	runtime        *DaemonRuntime
	actions        sys.Actions
	reloadConfig   func() error
	statusProvider func() string
}

func NewServer(rt *DaemonRuntime, actions sys.Actions, reloadConfig func() error) *Server {
	if actions == nil {
		actions = sys.RealActions{}
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
	fmt.Printf("Received ==> %+v\n", req)

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
			return protocol.Response{
				Type: "error",
				Error: &protocol.ErrorResponse{
					Message: "missing start payload",
				},
			}
		}
		return s.handleStart(*req.Start)
	case "status":
		return s.handleStatus()
	case "cancel":
		return s.handleCancel()
	case "history":
		return s.handleHistory()
	case "reload":
		return s.handleReload()
	default:
		fmt.Printf("Unknown command: %s\n", req.Command)
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: fmt.Sprintf("Unknown command: %s", req.Command),
			},
		}
	}
}

func (s *Server) handleReload() protocol.Response {
	if s.reloadConfig == nil {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: "reload is not available",
			},
		}
	}
	if err := s.reloadConfig(); err != nil {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: fmt.Sprintf("reload failed: %v", err),
			},
		}
	}
	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: "Config reloaded.",
		},
	}
}

func (s *Server) handleStart(req protocol.StartRequest) protocol.Response {
	duration := req.Duration
	if req.Preset != "" {
		resolvedDuration, err := state.ResolveTaskPresetDuration(req.Preset)
		if err != nil {
			return protocol.Response{
				Type: "error",
				Error: &protocol.ErrorResponse{
					Message: err.Error(),
				},
			}
		}
		duration = resolvedDuration
	}
	if duration <= 0 {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: "missing task duration; provide a preset (short|medium|long|deep)",
			},
		}
	}

	task, err := s.runtime.StartTask(req.Title, duration)
	if err != nil {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: err.Error(),
			},
		}
	}
	s.actions.Notify("Task Started", fmt.Sprintf("Started task: %s for %s", task.Title, task.Duration))
	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: fmt.Sprintf("Started task: %s for %s", task.Title, task.Duration),
		},
	}
}

func startDecisionFromCore(snapshot core.State) error {
	switch snapshot.Phase {
	case core.PhaseIdle:
		return nil
	case core.PhasePendingCooldown, core.PhaseCooldown:
		return fmt.Errorf("cooldown active, wait before creating a new task")
	case core.PhaseBreak:
		return fmt.Errorf("break active, wait before creating a new task")
	default:
		return fmt.Errorf("a task is already active")
	}
}

func (s *Server) handleStatus() protocol.Response {
	message := s.runtime.Status()
	if s.statusProvider != nil {
		message = s.statusProvider()
	}
	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: message,
		},
	}
}

func (s *Server) handleCancel() protocol.Response {
	if err := cancelDecisionFromCore(s.runtime.CoreSnapshot()); err != nil {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: fmt.Sprintf("Failed to cancel task: %v", err),
			},
		}
	}

	task, err := s.runtime.CancelCurrentTask()
	if err != nil {
		return protocol.Response{
			Type: "error",
			Error: &protocol.ErrorResponse{
				Message: fmt.Sprintf("Failed to cancel task: %v", err),
			},
		}
	}
	s.actions.Notify("Task Cancelled", fmt.Sprintf("Cancelled the task: %s", task.Title))
	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: fmt.Sprintf("Cancelled the task: %s", task.Title),
		},
	}
}

func cancelDecisionFromCore(snapshot core.State) error {
	switch snapshot.Phase {
	case core.PhaseIdle:
		return fmt.Errorf("no active task to cancel")
	case core.PhasePendingCooldown, core.PhaseCooldown:
		return fmt.Errorf("no active task to cancel")
	default:
		// Active/Break are allowed here; legacy path still enforces grace-period lock.
		return nil
	}
}

func (s *Server) handleHistory() protocol.Response {
	history := s.runtime.History()
	if len(history) == 0 {
		return protocol.Response{
			Type: "success",
			Success: &protocol.SuccessResponse{
				Message: "No task history",
			},
		}
	}

	lines := make([]string, 0, len(history))
	for _, task := range history {
		lines = append(lines, formatHistoryLine(task))
	}

	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: strings.Join(lines, "\n"),
		},
	}
}

func formatHistoryLine(task state.Task) string {
	return fmt.Sprintf(
		"[%d] %s | %s | %s | started %s",
		task.ID,
		task.Title,
		task.Duration.Round(time.Second),
		task.Status,
		task.StartTime.Format(time.RFC3339),
	)
}
