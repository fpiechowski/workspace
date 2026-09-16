package tui

import (
	"context"
	"os/exec"

	"workspace/internal/core"
)

type Backend interface {
	ProjectOverview(context.Context) (core.ProjectOverview, error)
	WorkspaceSnapshot(context.Context, string) (core.WorkspaceSnapshot, error)
	ObserveWorkspaceRuntime(context.Context, string) (core.RuntimeObservation, error)
	InspectWorktree(context.Context, string, string) (core.WorktreeObservation, error)
	ReadPreview(context.Context, string, core.PreviewKind, string) (core.Preview, error)
	ResolveNavigationTarget(context.Context, string, core.EntityRef) (core.NavigationTarget, error)
}

type Navigator interface {
	Select(context.Context, core.NavigationTarget) error
	PrepareAttach(context.Context, core.NavigationTarget) (*exec.Cmd, error)
}

type ActionCall struct {
	NavigationRef      *core.EntityRef
	OpenTerminal       bool
	Action             string
	TargetID           string
	WorkspaceID        string
	TargetName         string
	TargetDetails      string
	Reason             string
	Input              string
	Workflow           string
	Key                string
	ExpectedRevision   int
	ExpectedRunID      string
	ExpectedAttempt    int
	ExpectedRunIDs     []string
	ExpectedServiceIDs []string
}

type ActionBackend interface {
	PerformAction(context.Context, string, ActionCall) error
}

type ManagedUIHider interface {
	HideManagedUI(context.Context, string, string) error
}

type WorkflowBackend interface {
	WorkflowNames(context.Context, string) ([]string, error)
}

type UIStatusBackend interface {
	UIPaneStatus(context.Context, string) (core.UIStatus, error)
}

type CoreBackend struct{ Service *core.Service }

func (b CoreBackend) ProjectOverview(ctx context.Context) (core.ProjectOverview, error) {
	return b.Service.ProjectOverview(ctx)
}
func (b CoreBackend) WorkspaceSnapshot(ctx context.Context, selector string) (core.WorkspaceSnapshot, error) {
	return b.Service.WorkspaceSnapshot(ctx, selector)
}
func (b CoreBackend) ObserveWorkspaceRuntime(ctx context.Context, selector string) (core.RuntimeObservation, error) {
	return b.Service.ObserveWorkspaceRuntime(ctx, selector)
}
func (b CoreBackend) InspectWorktree(ctx context.Context, selector, id string) (core.WorktreeObservation, error) {
	return b.Service.InspectWorktree(ctx, selector, id)
}
func (b CoreBackend) ReadPreview(ctx context.Context, selector string, kind core.PreviewKind, id string) (core.Preview, error) {
	return b.Service.ReadPreview(ctx, selector, kind, id)
}
func (b CoreBackend) ResolveNavigationTarget(ctx context.Context, selector string, ref core.EntityRef) (core.NavigationTarget, error) {
	return b.Service.ResolveNavigationTarget(ctx, selector, ref)
}
func (b CoreBackend) PerformAction(ctx context.Context, selector string, call ActionCall) error {
	switch call.Action {
	case "start_orchestrator":
		_, err := b.Service.StartSupervisedOrchestrator(ctx, selector, call.Key)
		return err
	case "create_workspace":
		_, err := b.Service.Create(ctx, core.CreateOptions{Title: call.TargetName, Input: call.Input, Workflow: call.Workflow, OperationKey: call.Key})
		return err
	case "delete_workspace":
		return b.Service.DeleteWorkspace(ctx, call.TargetID, call.Key, call.ExpectedRevision)
	case "archive_workspace":
		_, err := b.Service.ArchiveGuarded(ctx, selector, call.Key, call.ExpectedRevision)
		return err
	case "pause":
		_, err := b.Service.SetPausedGuarded(ctx, selector, true, call.Key, core.MutationGuard{ExpectedRevision: call.ExpectedRevision})
		return err
	case "pause_interrupt":
		_, err := b.Service.PauseInterruptGuarded(ctx, selector, call.Key, core.MutationGuard{
			ExpectedRevision: call.ExpectedRevision, ExpectedRunIDs: call.ExpectedRunIDs, ExpectedServiceIDs: call.ExpectedServiceIDs,
		})
		return err
	case "resume_workspace":
		_, err := b.Service.SetPausedGuarded(ctx, selector, false, call.Key, core.MutationGuard{ExpectedRevision: call.ExpectedRevision})
		return err
	case "reconcile":
		_, err := b.Service.ReconcileWorkspace(ctx, selector, call.Key)
		return err
	case "resume_session":
		_, err := b.Service.ResumeSession(ctx, selector, call.TargetID, call.Key)
		return err
	case "stop_run":
		_, err := b.Service.StopRun(ctx, selector, call.TargetID, call.ExpectedRunID, call.Key)
		return err
	case "close_session":
		_, err := b.Service.CloseSession(ctx, selector, call.TargetID, call.Reason, call.Key)
		return err
	case "retry_task":
		_, err := b.Service.RetryTaskGuarded(ctx, selector, call.TargetID, call.Reason, call.Key, core.MutationGuard{ExpectedRevision: call.ExpectedRevision, ExpectedAttempt: call.ExpectedAttempt})
		return err
	case "delete_task":
		_, err := b.Service.DeleteTask(ctx, selector, call.TargetID, call.Key, core.MutationGuard{ExpectedRevision: call.ExpectedRevision, ExpectedAttempt: call.ExpectedAttempt})
		return err
	case "delete_session":
		_, err := b.Service.DeleteSession(ctx, selector, call.TargetID, call.Key, core.MutationGuard{ExpectedRevision: call.ExpectedRevision, ExpectedRunID: call.ExpectedRunID})
		return err
	case "stop_service":
		_, err := b.Service.StopService(ctx, selector, call.TargetID, call.Key)
		return err
	case "select_workflow":
		_, err := b.Service.SelectWorkflowGuarded(ctx, selector, call.TargetID, call.Key, call.ExpectedRevision)
		return err
	case "show_managed_tui", "hide_managed_tui":
		_, err := b.Service.SetUIPaneDesired(ctx, selector, call.Action == "show_managed_tui", call.Key)
		return err
	default:
		return &core.Error{Code: "invalid_action", Message: "unsupported interactive action"}
	}
}

func (b CoreBackend) HideManagedUI(ctx context.Context, selector, key string) error {
	return b.Service.HideManagedUIPane(ctx, selector, key)
}

func (b CoreBackend) WorkflowNames(context.Context, string) ([]string, error) {
	config, err := b.Service.Config()
	if err != nil {
		return nil, err
	}
	return core.WorkflowNames(b.Service.Root, config), nil
}

func (b CoreBackend) UIPaneStatus(ctx context.Context, selector string) (core.UIStatus, error) {
	return b.Service.UIPaneStatus(ctx, selector)
}

type Config struct {
	ProjectRoot, ProjectID, CWD, WorkspaceID string
	ProjectFound                             bool
	InitialError                             string
	Theme                                    string
	NoColor                                  bool
	Managed                                  bool
	Backend                                  Backend
	Navigator                                Navigator
}
