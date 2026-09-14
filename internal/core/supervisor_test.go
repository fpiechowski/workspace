package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSupervisorRecoversOnlyVerifiedLostOrchestrator(t *testing.T) {
	s, id := fixture(t)
	ctx := context.Background()
	p, err := s.StartOrchestrator(ctx, id, "initial")
	if err != nil {
		t.Fatal(err)
	}
	rt := s.Runtime.(*fakeRuntime)
	rt.inspectError = fail("tmux_error", "temporarily unavailable")
	if err := s.Tick(ctx); err == nil {
		t.Fatal("outage not reported")
	}
	if rt.launches != 1 {
		t.Fatal("runtime outage duplicated orchestrator")
	}
	rt.inspectError = nil
	delete(rt.panes, p.PaneID)
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := s.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Sessions) != 1 || len(v.Runs) != 2 || v.Sessions[0].AgentID != p.AgentID || v.Runs[0].State != "interrupted" || v.Sessions[0].CurrentRunID != v.Runs[1].ID {
		t.Fatalf("recovery: %+v", v.Sessions)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.launches != 2 {
		t.Fatal("recovery launched duplicate")
	}
	if _, err := s.StopSession(ctx, id, v.Sessions[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.launches != 2 {
		t.Fatal("explicitly stopped session restarted")
	}
}

func TestSupervisorSocketLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket runtime")
	}
	s, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- s.Serve(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	var info SupervisorInfo
	for {
		var err error
		info, err = s.SupervisorStatus(ctx)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info.PID != os.Getpid() || info.Token != "" {
		t.Fatal("status leaks token or wrong process")
	}
	if err := s.Serve(ctx); err == nil {
		t.Fatal("second supervisor allowed")
	}
	if err := s.StopSupervisor(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".workspace", ".runtime", "supervisor.json")); !os.IsNotExist(err) {
		t.Fatal("stale supervisor descriptor")
	}
}
