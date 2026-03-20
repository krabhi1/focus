package main

import (
	"encoding/gob"
	"fmt"
	"focus/internal/protocol"
	"focus/internal/state"
	"focus/internal/sys"
	"net"
	"strings"
	"time"
)

type Server struct {
	state        *state.DaemonState
	actions      sys.Actions
	reloadConfig func() error
}

func NewServer(st *state.DaemonState, actions sys.Actions, reloadConfig func() error) *Server {
	if actions == nil {
		actions = sys.RealActions{}
	}
	return &Server{state: st, actions: actions, reloadConfig: reloadConfig}
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
	task, err := s.state.NewTask(req.Title, req.Duration)
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

func (s *Server) handleStatus() protocol.Response {
	return protocol.Response{
		Type: "success",
		Success: &protocol.SuccessResponse{
			Message: s.state.GetStatus(),
		},
	}
}

func (s *Server) handleCancel() protocol.Response {
	task, err := s.state.CancelCurrentTask()
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

func (s *Server) handleHistory() protocol.Response {
	history := s.state.History()
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
