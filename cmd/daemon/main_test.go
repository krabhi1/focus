package main

import (
	"encoding/gob"
	"focus/internal/protocol"
	"focus/internal/state"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConnectionStartStatusCooldownFlow(t *testing.T) {
	restorePolicy := state.SetCooldownDurationPolicyForTest(func(time.Duration) time.Duration {
		return 200 * time.Millisecond
	})
	t.Cleanup(restorePolicy)

	state.Get().ResetForTest()
	t.Cleanup(state.Get().ResetForTest)

	res := roundTripRequest(t, protocol.Request{
		Command: "start",
		Payload: protocol.StartRequest{
			Title:    "integration task",
			Duration: 10 * time.Millisecond,
		},
	})
	assertSuccessMessageContains(t, res, "Started task: integration task")

	time.Sleep(40 * time.Millisecond)

	statusRes := roundTripRequest(t, protocol.Request{Command: "status"})
	statusPayload, ok := statusRes.Payload.(protocol.SuccessResponse)
	if !ok {
		t.Fatalf("payload = %#v, want success response", statusRes.Payload)
	}
	if !strings.Contains(statusPayload.Message, "Cooldown active") {
		t.Fatalf("status = %q, want cooldown", statusPayload.Message)
	}

	cooldownRes := roundTripRequest(t, protocol.Request{
		Command: "start",
		Payload: protocol.StartRequest{
			Title:    "blocked task",
			Duration: 10 * time.Millisecond,
		},
	})
	errorPayload, ok := cooldownRes.Payload.(protocol.ErrorResponse)
	if !ok {
		t.Fatalf("payload = %#v, want error response", cooldownRes.Payload)
	}
	if !strings.Contains(errorPayload.Message, "cooldown active") {
		t.Fatalf("error = %q, want cooldown rejection", errorPayload.Message)
	}

	time.Sleep(250 * time.Millisecond)

	retryRes := roundTripRequest(t, protocol.Request{
		Command: "start",
		Payload: protocol.StartRequest{
			Title:    "second task",
			Duration: 10 * time.Millisecond,
		},
	})
	assertSuccessMessageContains(t, retryRes, "Started task: second task")
}

func roundTripRequest(t *testing.T, req protocol.Request) protocol.Response {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleConnection(serverConn)
	}()
	defer func() {
		_ = clientConn.Close()
		<-done
	}()

	if err := gob.NewEncoder(clientConn).Encode(req); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var res protocol.Response
	if err := gob.NewDecoder(clientConn).Decode(&res); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	return res
}

func assertSuccessMessageContains(t *testing.T, res protocol.Response, want string) {
	t.Helper()

	successPayload, ok := res.Payload.(protocol.SuccessResponse)
	if !ok {
		t.Fatalf("payload = %#v, want success response", res.Payload)
	}
	if !strings.Contains(successPayload.Message, want) {
		t.Fatalf("message = %q, want substring %q", successPayload.Message, want)
	}
}
