package main

import "testing"

func TestBuildStartRequestIncludesNoBreakFlag(t *testing.T) {
	req, err := buildStartRequest([]string{"--name", "demo", "--duration", "long", "--no-break"})
	if err != nil {
		t.Fatalf("buildStartRequest returned error: %v", err)
	}
	if req.Start == nil {
		t.Fatal("Start = nil, want payload")
	}
	if !req.Start.NoBreak {
		t.Fatal("NoBreak = false, want true")
	}
}

func TestBuildStartRequestDefaultsNoBreakToFalse(t *testing.T) {
	req, err := buildStartRequest([]string{"--name", "demo", "--duration", "long"})
	if err != nil {
		t.Fatalf("buildStartRequest returned error: %v", err)
	}
	if req.Start == nil {
		t.Fatal("Start = nil, want payload")
	}
	if req.Start.NoBreak {
		t.Fatal("NoBreak = true, want false")
	}
}
