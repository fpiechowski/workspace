package core

import (
	"fmt"
	"strings"
)

// Workflow capabilities are persisted with the workflow snapshot so changing
// project configuration cannot silently change the contract of an in-flight
// workspace. Older snapshots use the workflow-id defaults below.
const (
	capTasks          = "tasks"
	capTaskRoles      = "tasks.roles"
	capPlanner        = "tasks.role.planner"
	capImplementer    = "tasks.role.implementer"
	capIntegrator     = "tasks.role.integrator"
	capTester         = "tasks.role.tester"
	capPlannerDepends = "planner_dependency"
	capPhases         = "phases"
	capIntegration    = "integration"
	capLiveTest       = "live_test"
	capChangeRequest  = "change_request"
	capRelease        = "release"
	capLanding        = "landing"
)

// legacySnapshotCapabilities is the fallback for persisted snapshots that
// predate capability persistence (an empty Capabilities list). It keeps the v1
// semantics of each bundled workflow so an in-flight snapshot never silently
// changes contract. A v1 plan-first workspace therefore still completes after
// implementation instead of gaining the integration stage.
func legacySnapshotCapabilities(id string) []string {
	base := []string{capTasks, capPhases, capPlanner, capPlannerDepends}
	switch id {
	case "plan-first":
		return append(base, capImplementer)
	case "issue-resolution":
		return append(base, capImplementer, capIntegrator, capTester, capIntegration, capLiveTest, capChangeRequest, capRelease)
	default:
		return nil
	}
}

// builtinWorkflowCapabilities is the required capability set for a new
// selection of a bundled workflow. plan-first is v2: mandatory integration and
// a user-approved landing gate.
func builtinWorkflowCapabilities(id string) []string {
	if id == "plan-first" {
		return []string{capTasks, capPhases, capPlanner, capImplementer, capIntegrator, capPlannerDepends, capIntegration, capLanding}
	}
	return nil
}

func workflowCapabilities(d *Document) []string {
	if d == nil || d.State.Workflow == nil {
		return nil
	}
	if len(d.State.Workflow.Capabilities) > 0 {
		return d.State.Workflow.Capabilities
	}
	return legacySnapshotCapabilities(d.State.Workflow.ID)
}

func workflowHasCapability(d *Document, capability string) bool {
	for _, candidate := range workflowCapabilities(d) {
		if candidate == capability {
			return true
		}
		if candidate == capTaskRoles && (capability == capTasks || strings.HasPrefix(capability, "tasks.role.")) {
			return true
		}
	}
	return false
}

func requireWorkflowCapability(d *Document, capability string) error {
	if d == nil || d.State.Workflow == nil {
		if d != nil && d.State.NeedsWorkflow() {
			return decisionRequired("select a workflow", "plan-first")
		}
		return fail("workflow_capability", "operation requires workflow capability %s, but no workflow is selected", capability)
	}
	if workflowHasCapability(d, capability) {
		return nil
	}
	return fail("workflow_capability", "workflow %s does not declare capability %s", d.State.Workflow.ID, capability)
}

func requireWorkflowRole(d *Document, role string) error {
	if err := requireWorkflowCapability(d, capTasks); err != nil {
		return err
	}
	capability := fmt.Sprintf("tasks.role.%s", role)
	if err := requireWorkflowCapability(d, capability); err != nil {
		return err
	}
	return nil
}

// workflowConfigCapabilities returns the capability set a new selection
// snapshots. A plan-first configuration can only add to the required built-in
// v2 set: the mandatory integration stage cannot be dropped by a project
// config that still lists the v1 capabilities. Other workflow ids keep
// "configured, else nothing" because their capabilities are project-defined.
func workflowConfigCapabilities(id string, cfg Config) []string {
	configured := cfg.Workflows[id].Capabilities
	if id == "plan-first" {
		return unionCapabilities(configured, builtinWorkflowCapabilities(id))
	}
	if len(configured) > 0 {
		return append([]string(nil), configured...)
	}
	return nil
}

func unionCapabilities(configured, required []string) []string {
	out := make([]string, 0, len(configured)+len(required))
	seen := map[string]bool{}
	for _, list := range [][]string{configured, required} {
		for _, capability := range list {
			if capability == "" || seen[capability] {
				continue
			}
			seen[capability] = true
			out = append(out, capability)
		}
	}
	return out
}

func knownWorkflowCapability(capability string) bool {
	if capability == capTasks || capability == capTaskRoles || capability == capPhases || capability == capPlannerDepends ||
		capability == capIntegration || capability == capLiveTest || capability == capChangeRequest || capability == capRelease ||
		capability == capLanding {
		return true
	}
	return capability == capPlanner || capability == capImplementer || capability == capIntegrator || capability == capTester
}

// WorkflowCapabilitiesForList exposes the same effective capability snapshot
// used by task and resource mutations for the CLI/UI discovery contract.
func WorkflowCapabilitiesForList(id string, cfg Config) []string {
	return workflowConfigCapabilities(id, cfg)
}
