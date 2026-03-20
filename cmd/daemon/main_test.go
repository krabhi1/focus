package main

import (
	"encoding/gob"
	"focus/internal/protocol"
	"focus/internal/state"
	"focus/internal/sys"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnectionStartStatusCooldownFlow(t *testing.T) {
	st := newTestState(t)

	socketPath := filepath.Join(t.TempDir(), "focus.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})

	acceptDone := make(chan struct{})
	srv := NewServer(st, sys.NoopActions{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go srv.HandleConnection(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
	})

	res := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:    "integration task",
			Duration: 10 * time.Millisecond,
		},
	})
	assertSuccessMessageContains(t, res, "Started task: integration task")

	time.Sleep(40 * time.Millisecond)

	statusRes := roundTripRequest(t, socketPath, protocol.Request{Command: "status"})
	if statusRes.Success == nil {
		t.Fatalf("response = %#v, want success response", statusRes)
	}
	if !strings.Contains(statusRes.Success.Message, "Cooldown active") {
		t.Fatalf("status = %q, want cooldown", statusRes.Success.Message)
	}

	cooldownRes := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:    "blocked task",
			Duration: 10 * time.Millisecond,
		},
	})
	if cooldownRes.Error == nil {
		t.Fatalf("response = %#v, want error response", cooldownRes)
	}
	if !strings.Contains(cooldownRes.Error.Message, "cooldown active") {
		t.Fatalf("error = %q, want cooldown rejection", cooldownRes.Error.Message)
	}

	time.Sleep(250 * time.Millisecond)

	retryRes := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:    "second task",
			Duration: 10 * time.Millisecond,
		},
	})
	assertSuccessMessageContains(t, retryRes, "Started task: second task")

	historyRes := roundTripRequest(t, socketPath, protocol.Request{Command: "history"})
	if historyRes.Success == nil {
		t.Fatalf("response = %#v, want success response", historyRes)
	}
	if !strings.Contains(historyRes.Success.Message, "integration task") {
		t.Fatalf("history = %q, want first task", historyRes.Success.Message)
	}
	if !strings.Contains(historyRes.Success.Message, "second task") {
		t.Fatalf("history = %q, want second task", historyRes.Success.Message)
	}
}

func newTestState(t *testing.T) *state.DaemonState {
	t.Helper()

	st := &state.DaemonState{}
	st.SetActions(sys.NoopActions{})
	st.SetCooldownPolicyForTest(func(time.Duration) time.Duration {
		return 200 * time.Millisecond
	})

	t.Cleanup(func() {
		st.SetActions(sys.RealActions{})
		st.SetCooldownPolicyForTest(nil)
	})

	return st
}

func roundTripRequest(t *testing.T, socketPath string, req protocol.Request) protocol.Response {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var res protocol.Response
	if err := gob.NewDecoder(conn).Decode(&res); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	return res
}

func assertSuccessMessageContains(t *testing.T, res protocol.Response, want string) {
	t.Helper()

	if res.Success == nil {
		t.Fatalf("response = %#v, want success response", res)
	}
	if !strings.Contains(res.Success.Message, want) {
		t.Fatalf("message = %q, want substring %q", res.Success.Message, want)
	}
}
