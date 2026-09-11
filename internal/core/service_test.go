package core

import (
	"context"
	"os"
	"testing"
)

func TestServicesAreSeparateAndPreventCleanup(t *testing.T) {
	s, ws := fixture(t)
	ctx := context.Background()
	_, w := worker(t, s, ws, "app")
	exe, _ := os.Executable()
	opt := ServiceOptions{Name: "app", Worktree: w.ID, Argv: []string{exe, "-test.run=TestWorkerProcess"}, OperationKey: "app:1"}
	p, err := s.StartService(ctx, ws, opt)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.StartService(ctx, ws, opt)
	if err != nil || replay.ID != p.ID {
		t.Fatal("service duplicated", err)
	}
	v, err := s.Status(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Sessions) != 0 {
		t.Fatal("service represented as agent Session")
	}
	if err := s.With(ctx, ws, func(d *Document) error {
		d.State.Status = "completed"
		d.State.Release.UserConfirmed = true
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Archive(ctx, ws)
	expectCode(t, err, "service_active")
	delete(s.Runtime.(*fakeRuntime).panes, p.PaneID)
	if _, err := s.Reconcile(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(ctx, ws); err != nil {
		t.Fatal(err)
	}
}
