package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// helpUsageTemplate keeps the shape of every help page predictable. Cobra's
// default template only has a single Usage section, which makes positional
// arguments easy to miss on commands with several options.
const helpUsageTemplate = `Usage:
{{- if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if .HasExample}}

Examples:
{{.Example | trimTrailingWhitespaces}}{{end}}{{with (index .Annotations "workspace.arguments")}}

Arguments:
{{. | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Commands:
{{- range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Options:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global options:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

type helpArgument struct {
	name        string
	description string
	required    bool
}

type commandHelp struct {
	long      string
	arguments []helpArgument
	example   string
}

func h(long string, example string, arguments ...helpArgument) commandHelp {
	return commandHelp{long: long, example: example, arguments: arguments}
}

func requiredArgument(name, description string) helpArgument {
	return helpArgument{name: name, description: description, required: true}
}

func optionalArgument(name, description string) helpArgument {
	return helpArgument{name: name, description: description}
}

// commandHelpSpecs is deliberately keyed by the rendered command path. It is
// easy to audit in one place and the test suite verifies that every visible
// command has an entry.
var commandHelpSpecs = map[string]commandHelp{
	"workspace": h(
		"Manage durable agent workflows, personas, sessions, worktrees and their recorded results. Use a command's local help to see its arguments and options.",
		"workspace help session start",
	),
	"workspace help": h(
		"Show help for the application or for a command path. Use the command's --help flag or append help when that is more convenient.",
		"workspace help session start",
		optionalArgument("command", "Command path to document, such as session start; omit it to show application help."),
	),
	"workspace project": h(
		"Configure a Git project for workspace orchestration. Project configuration and editable workflow templates live under .workspace.",
		"workspace project init",
	),
	"workspace project init": h(
		"Initialize workspace metadata, default configuration and workflow templates in the current Git project. Existing configuration is preserved.",
		"workspace project init --project ./repo",
	),
	"workspace skill": h(
		"Install the bundled workspace skill where the selected client can discover it.",
		"workspace skill install --client codex",
	),
	"workspace skill install": h(
		"Copy the workspace skill into the project-specific discovery directory. Existing modified skill files are not overwritten.",
		"workspace skill install --client codex",
	),
	"workspace version": h(
		"Show the embedded version, source commit, build date and runtime target. This command works from any directory and does not inspect project state.",
		"workspace version --json",
	),
	"workspace prime": h(
		"Print the current binary's bundled general agent guidance as raw Markdown, for use at a new session or after context compaction. This command works without a project and does not report live workspace state; use status or menu for that.",
		"workspace prime\nworkspace prime --json",
	),
	"workspace upgrade": h(
		"Download the newest stable release for the current supported target, verify its checksum and archive layout, then atomically replace the running executable. Existing supervisors and tmux processes keep their old in-memory code until restarted.",
		"workspace upgrade",
	),
	"workspace doctor": h(
		"Inspect Git, tmux, project configuration and the local execution environment. Use --json when another tool will consume the result.",
		"workspace doctor --json",
	),
	"workspace create": h(
		"Create a workspace from an issue URL, a saved input file or one inline intent. One input source is required; the input and its source are snapshotted before orchestration begins. Omit --workflow to choose a workflow later, or pass --no-workflow for explicit manual orchestration.",
		"workspace create --issue https://github.com/OWNER/REPO/issues/142 --workflow plan-first\nworkspace create \"Improve workspace creation\" --no-workflow",
		optionalArgument("intent", "Issue or task description supplied inline; do not combine it with --input-file."),
	),
	"workspace issue": h(
		"Manage durable project Issues. Issue content is local input data; tracker adapters remain read-only and linked Workspace inputs are frozen revisions.",
		"workspace issue list\nworkspace issue create --issue https://tracker.example/issues/142",
	),
	"workspace issue create": h(
		"Create a project Issue from one manual description, file, or HTTP(S) source. A source URL is fetched only when no local body is supplied.",
		"workspace issue create \"Improve retry handling\" --title \"Retry handling\"",
		optionalArgument("intent", "Manual Issue description; do not combine it with --input-file."),
	),
	"workspace issue list": h(
		"List bounded Issue summaries, local status, revision and derived linked Workspace counts without loading full bodies.",
		"workspace issue list --short",
	),
	"workspace issue show": h(
		"Show one Issue's canonical content, revision metadata and derived linked Workspaces.",
		"workspace issue show issue_01",
		requiredArgument("issue", "Durable Issue ID."),
	),
	"workspace issue refresh": h(
		"Refresh a sourced Issue through the configured read-only tracker. Tracker failures never create guessed content.",
		"workspace issue refresh issue_01 --expected-revision 2 --operation-key refresh:issue_01:2",
		requiredArgument("issue", "Durable sourced Issue ID."),
	),
	"workspace issue update": h(
		"Change only the local Issue status with an exact revision guard. Deferred and closed statuses require a reason.",
		"workspace issue update issue_01 --status deferred --reason \"Waiting for product decision\" --expected-revision 2",
		requiredArgument("issue", "Durable Issue ID."),
	),
	"workspace issue dispatch": h(
		"Create a Workspace from the selected frozen Issue revision and optionally start its Workspace Orchestrator.",
		"workspace issue dispatch issue_01 --workflow plan-first --start --operation-key dispatch:issue_01",
		requiredArgument("issue", "Durable Issue ID."),
	),
	"workspace dispatcher": h(
		"Manage the singleton project-scoped Dispatcher. It handles project Issue intake and routing only; Workspace Orchestrators own execution.",
		"workspace dispatcher status\nworkspace dispatcher start --profile dispatcher",
	),
	"workspace dispatcher start": h(
		"Explicitly start or resume the project Dispatcher. The profile falls back from defaults.dispatcher_profile to defaults.orchestrator_profile.",
		"workspace dispatcher start --profile dispatcher --operation-key dispatcher:start",
	),
	"workspace dispatcher status": h(
		"Read Dispatcher Agent, logical Sessions, Runs and verified project-scope tmux ownership without starting runtime state.",
		"workspace dispatcher status --json",
	),
	"workspace dispatcher stop": h(
		"Stop the current Dispatcher Run idempotently and persist an explicit no-auto-restart intent.",
		"workspace dispatcher stop --operation-key dispatcher:stop",
	),
	"workspace dispatcher attach": h(
		"Attach to the exact verified project Dispatcher tmux session and pane.",
		"workspace dispatcher attach",
	),
	"workspace list": h(
		"List workspaces known to this project. Use --short (or --map) for a compact name-to-ID map.",
		"workspace list --short",
	),
	"workspace tui": h(
		"Open the interactive project and workspace browser. Requires a terminal on both stdin and stdout. The managed companion panel is controlled by workspace tui show, hide and status.",
		"workspace tui --project ./repo --theme dark",
	),
	"workspace tui show": h(
		"Explicitly enable and reconcile the managed companion panel into an existing orchestrator window. --operation-key makes retries idempotent.",
		"workspace tui show --workspace ws_01 --operation-key tui-show:ws_01",
	),
	"workspace tui hide": h(
		"Disable and remove only the verified managed companion panel. --operation-key makes retries idempotent.",
		"workspace tui hide --workspace ws_01 --operation-key tui-hide:ws_01",
	),
	"workspace tui status": h(
		"Show desired state, generation, pane/window ownership and last known runtime state for the managed companion panel.",
		"workspace tui status --workspace ws_01 --json",
	),
	"workspace open": h(
		"Choose a workspace interactively when no selector is supplied, then attach to its tmux session. A selector can be an ID or title.",
		"workspace open\nworkspace open specification",
		optionalArgument("workspace", "Workspace ID or title; omit it to show the interactive selector."),
	),
	"workspace status": h(
		"Show the durable workspace document, workflow phase, agents, sessions, tasks, worktrees and recorded results.",
		"workspace status --workspace ws_01",
	),
	"workspace workflow": h(
		"Inspect available workflows and choose or advance the workflow associated with the selected workspace.",
		"workspace workflow list",
	),
	"workspace workflow list": h(
		"List workflow names available from project configuration and bundled templates.",
		"workspace workflow list",
	),
	"workspace workflow select": h(
		"Select a workflow for a workspace that is waiting for a workflow decision. The selection is recorded with revision and operation metadata.",
		"workspace workflow select plan-first",
		requiredArgument("name", "Configured workflow name, such as plan-first or issue-resolution."),
	),
	"workspace workflow advance": h(
		"Validate the current workflow results and advance exactly one phase. --to can protect against advancing from an unexpected phase.",
		"workspace workflow advance --to implementation",
	),
	"workspace workflow migrate": h(
		"Apply updated workflow templates while preserving input history and invalidating results affected by the revision.",
		"workspace workflow migrate --reason \"Refresh planning instructions\"",
	),
	"workspace input": h(
		"Revise the issue input while preserving its previous snapshots and invalidating results that depend on the changed revision.",
		"workspace input update --input-file revised-issue.md --reason \"Clarify acceptance criteria\"",
	),
	"workspace input update": h(
		"Replace the current issue description with a file or tracker snapshot. One input source is required; the previous input remains available in revision history.",
		"workspace input update --input-file revised-issue.md --expected-revision 3",
	),
	"workspace profile": h(
		"Inspect model-routing profiles configured for this project.",
		"workspace profile list",
	),
	"workspace profile list": h(
		"List configured model profiles, routes and concurrency settings.",
		"workspace profile list",
	),
	"workspace profile explain": h(
		"Explain route eligibility, provider balancing and recent launch pressure for one profile.",
		"workspace profile explain worker",
		requiredArgument("name", "Configured profile name to explain."),
	),
	"workspace client": h(
		"Inspect client adapters configured for this project.",
		"workspace client list",
	),
	"workspace client list": h(
		"List configured Codex, Claude, OpenCode or command client definitions.",
		"workspace client list",
	),
	"workspace agent": h(
		"Manage durable persona definitions. A persona can be resumed into multiple logical Sessions and concrete Runs.",
		"workspace agent list",
	),
	"workspace agent create": h(
		"Define a persona with a role, default profile and optional prompt or instruction files.",
		"workspace agent create planner --role planner --profile thinker",
		requiredArgument("name", "Stable persona name or display name."),
	),
	"workspace agent list": h(
		"List persona definitions and their associated sessions.",
		"workspace agent list",
	),
	"workspace agent resume": h(
		"Start a new concrete Run for a persona, reusing a compatible logical Session, worktree and profile when possible. In a completed workspace, an accepted worker resume is consultation-only; new work requires workspace reopen.",
		"workspace agent resume planner",
		requiredArgument("agent", "Agent ID or persona name to resume."),
	),
	"workspace worktree": h(
		"Manage dedicated Git checkouts used by delegated tasks.",
		"workspace worktree list",
	),
	"workspace worktree create": h(
		"Create a named branch and worktree for a task. The default base is the frozen workspace revision.",
		"workspace worktree create checkout-api --purpose implementation",
		requiredArgument("name", "Worktree name used to identify the checkout and branch."),
	),
	"workspace worktree list": h(
		"List task worktrees, their branches, purposes and recorded ownership.",
		"workspace worktree list",
	),
	"workspace session": h(
		"Manage durable logical Sessions and their concrete client/tmux Runs.",
		"workspace session list",
	),
	"workspace session start": h(
		"Launch a persona in a tmux pane. The logical Session records lineage; its Run records the selected route, client, prompt and pane.",
		"workspace session start --agent planner --parent-session sess_orchestrator --task task_01 --worktree planning",
	),
	"workspace session bind-thread": h(
		"Associate a native client conversation or session ID with an existing workspace session.",
		"workspace session bind-thread sess_01 --thread-id thread_abc",
		requiredArgument("session", "Workspace session ID to update."),
	),
	"workspace session list": h(
		"List logical Sessions with operational state (including live idle), lifecycle/run/client state, current/last Run, pane, native thread and lineage summaries.",
		"workspace session list",
	),
	"workspace session history": h(
		"List every concrete Run belonging to one logical Session, including terminated executions and their provenance.",
		"workspace session history sess_01",
		requiredArgument("session", "Logical Session ID, or a Run ID alias whose history should be shown."),
	),
	"workspace session resume": h(
		"Create a new concrete Run in a compatible logical Session after verifying its task, worktree, input and client lineage.",
		"workspace session resume sess_01",
		requiredArgument("session", "Logical Session ID, or a Run ID alias to resume."),
	),
	"workspace session stop": h(
		"Stop the current owned Run and its tmux pane while preserving the logical Session and run history.",
		"workspace session stop sess_01",
		requiredArgument("session", "Workspace session ID to stop."),
	),
	"workspace session close": h(
		"Close an idle logical Session so it cannot be resumed again. Existing Runs and provenance remain available.",
		"workspace session close sess_01 --reason completed",
		requiredArgument("session", "Logical Session ID to close."),
	),
	"workspace session attach": h(
		"Attach to the current Run's tmux pane after verifying logical Session and Run ownership.",
		"workspace session attach sess_01",
		requiredArgument("session", "Workspace session ID whose pane should receive focus."),
	),
	"workspace run": h(
		"Inspect concrete client and tmux executions independently from their durable logical Sessions.",
		"workspace run list",
	),
	"workspace run list": h(
		"List all concrete Runs across logical Sessions, including state, generation, route and pane ownership.",
		"workspace run list",
	),
	"workspace run inspect": h(
		"Inspect one concrete Run and its exact client, prompt, pane and lifecycle metadata.",
		"workspace run inspect run_01",
		requiredArgument("run", "Concrete Run ID to inspect."),
	),
	"workspace task": h(
		"Manage delegated tasks, dependencies, attempts and their acceptance state.",
		"workspace task list",
	),
	"workspace task create": h(
		"Create a task from a YAML or Markdown frontmatter specification file. The file must contain a title, goal, role and acceptance criteria.",
		"workspace task create --spec-file planning.yaml",
	),
	"workspace task list": h(
		"List task state, dependencies, attempts and required artifacts.",
		"workspace task list",
	),
	"workspace task retry": h(
		"Start a fresh attempt for a task and invalidate dependent results.",
		"workspace task retry task_01 --reason \"Previous check used stale fixtures\"",
		requiredArgument("task", "Task ID or task name to retry."),
	),
	"workspace message": h(
		"Send durable notes, questions, answers or status messages to exact logical Sessions.",
		"workspace message send --to-session sess_parent --kind question --body-file question.md",
	),
	"workspace message send": h(
		"Send the contents of a file as a durable message. Use --to-session for an exact logical Session; --to is compatibility-only and must resolve unambiguously.",
		"workspace message send --to-session sess_parent --kind note --body-file note.md",
	),
	"workspace inbox": h(
		"Read and acknowledge durable messages addressed to the current agent or orchestrator.",
		"workspace inbox list",
	),
	"workspace inbox list": h(
		"List pending messages for one exact Session. Use --agent explicitly for an agent-wide historical view, and --all to include acknowledged messages.",
		"workspace inbox list --session sess_parent --all",
	),
	"workspace inbox read": h(
		"Read one message without acknowledging it; --session selects the exact historical Session when run from the user terminal.",
		"workspace inbox read msg_01 --session sess_parent",
		requiredArgument("message", "Message ID to read."),
	),
	"workspace inbox ack": h(
		"Read and record receipt of one message for the exact target Session. Acknowledgement does not accept the associated task or handoff.",
		"workspace inbox ack msg_01 --session sess_parent",
		requiredArgument("message", "Message ID to acknowledge."),
	),
	"workspace inbox wait": h(
		"Wait for messages for one exact Session without holding the project lock. Use --agent explicitly for agent-wide history.",
		"workspace inbox wait --session sess_parent --timeout 60",
	),
	"workspace handoff": h(
		"Preserve work products and submit a result for orchestrator review.",
		"workspace handoff list",
	),
	"workspace handoff submit": h(
		"Copy explicitly named artifacts and enqueue a handoff result atomically. Use --to-session for the exact parent Session; a summary file is required.",
		"workspace handoff submit --to-session sess_parent --task task_01 --summary-file work-products/SUMMARY.md --artifact work-products/IMPLEMENTATION.md",
	),
	"workspace handoff list": h(
		"List submitted handoffs and their review state.",
		"workspace handoff list",
	),
	"workspace handoff accept": h(
		"Accept a submitted handoff and record the review decision separately from inbox acknowledgement.",
		"workspace handoff accept handoff_01",
		requiredArgument("handoff", "Handoff ID to accept."),
	),
	"workspace handoff reject": h(
		"Reject a submitted handoff and preserve review feedback for the next attempt.",
		"workspace handoff reject handoff_01 --reason-file feedback.md",
		requiredArgument("handoff", "Handoff ID to reject."),
	),
	"workspace artifact": h(
		"Inspect work products preserved by accepted or submitted handoffs.",
		"workspace artifact list",
	),
	"workspace artifact list": h(
		"List artifact paths, producing sessions and associated tasks.",
		"workspace artifact list",
	),
	"workspace integration": h(
		"Prepare a shared checkout and immutable input manifest from accepted implementation tasks.",
		"workspace integration prepare --tasks task_01,task_02",
	),
	"workspace integration prepare": h(
		"Create the integration worktree for accepted implementation tasks. Omit --tasks to include all accepted implementation tasks.",
		"workspace integration prepare --base main",
	),
	"workspace decision": h(
		"Refresh pending workflow questions and record explicit user answers.",
		"workspace decision answer decision_01 --answer \"Use the staging environment\" --user-confirmed",
	),
	"workspace decision refresh": h(
		"Replace a stale pending question while preserving the prior decision history.",
		"workspace decision refresh",
	),
	"workspace decision answer": h(
		"Record the user's answer to one pending decision revision.",
		"workspace decision answer decision_01 --answer yes --user-confirmed",
		requiredArgument("id", "Pending decision ID to answer."),
	),
	"workspace release": h(
		"Record explicit deployment or release confirmation for the workflow.",
		"workspace release confirm --reference production-2026-09-14 --user-confirmed",
	),
	"workspace release confirm": h(
		"Complete the workflow after the user explicitly confirms release. --reference is required and records the deployment or release identifier; agent sessions also require --user-confirmed.",
		"workspace release confirm --reference v1.4.0 --user-confirmed",
	),
	"workspace state": h(
		"Update durable workspace narrative state with optimistic revision checking.",
		"workspace state update --patch-file state.yaml --expected-revision 4",
	),
	"workspace state update": h(
		"Apply a typed YAML or JSON patch to title, body, status or phase and require the expected current revision.",
		"workspace state update --patch-file state.yaml --expected-revision 4",
	),
	"workspace state edit": h(
		"Edit a paused workspace narrative and title using EDITOR, or import a prepared WORKSPACE.md with --file.",
		"workspace state edit --file edited-WORKSPACE.md --expected-revision 4",
	),
	"workspace change-request": h(
		"Prepare, publish and reconcile external or local change-request records.",
		"workspace change-request prepare --title \"Improve workspace help\"",
	),
	"workspace change-request prepare": h(
		"Prepare a title, description and diff from a worktree without publishing it.",
		"workspace change-request prepare --worktree integration --target main --title \"Improve workspace help\"",
	),
	"workspace change-request publish": h(
		"Publish one prepared change request after explicit user confirmation. Reconcile an uncertain outcome before retrying.",
		"workspace change-request publish cr_01 --user-confirmed",
		requiredArgument("id", "Prepared change-request ID to publish."),
	),
	"workspace change-request sync": h(
		"Refresh external change-request state without creating or publishing a new request.",
		"workspace change-request sync",
	),
	"workspace change-request list": h(
		"List prepared, published and reconciled change-request records.",
		"workspace change-request list",
	),
	"workspace change-request skip": h(
		"Record the user's decision to skip publication of a prepared change request.",
		"workspace change-request skip cr_01 --reason \"Local delivery requested\" --user-confirmed",
		requiredArgument("id", "Prepared change-request ID to resolve."),
	),
	"workspace change-request link": h(
		"Record an existing external change-request URL for a prepared request.",
		"workspace change-request link cr_01 --url https://github.com/OWNER/REPO/pull/42 --user-confirmed",
		requiredArgument("id", "Prepared change-request ID to link."),
	),
	"workspace change-request retry": h(
		"Record the user's decision to retry publication after reconciling a failed or uncertain request.",
		"workspace change-request retry cr_01 --reason \"Forge was temporarily unavailable\" --user-confirmed",
		requiredArgument("id", "Change-request ID to retry."),
	),
	"workspace check": h(
		"Capture verification commands and immutable evidence receipts in the assigned worktree.",
		"workspace check run -- npm test",
	),
	"workspace check run": h(
		"Execute a verification command in the assigned worktree and record its exit code and evidence.",
		"workspace check run --session sess_01 -- go test ./...",
		requiredArgument("-- <command> [args...]", "Command to execute; place it after -- so its flags are not parsed by workspace."),
	),
	"workspace check list": h(
		"List captured check receipts, exit codes and evidence files.",
		"workspace check list",
	),
	"workspace service": h(
		"Run and inspect auxiliary processes in task worktree panes.",
		"workspace service list",
	),
	"workspace service start": h(
		"Start a named auxiliary process in a worktree pane and record its ownership.",
		"workspace service start dev-server --worktree checkout-api -- npm run dev",
		requiredArgument("name", "Stable service name."),
		requiredArgument("-- <command> [args...]", "Command to run; place it after --."),
	),
	"workspace service list": h(
		"List auxiliary service history, panes and exit states.",
		"workspace service list",
	),
	"workspace service stop": h(
		"Stop an owned auxiliary service pane while retaining its durable record.",
		"workspace service stop service_01",
		requiredArgument("id", "Service ID to stop."),
	),
	"workspace serve": h(
		"Run the project supervisor, which reconciles sessions and delivers durable messages. This is normally launched by workspace start.",
		"workspace serve",
	),
	"workspace server": h(
		"Inspect or stop the project supervisor without stopping agent processes.",
		"workspace server status",
	),
	"workspace server status": h(
		"Read the supervisor's current process and reconciliation status.",
		"workspace server status",
	),
	"workspace server stop": h(
		"Request supervisor shutdown while leaving agent processes running.",
		"workspace server stop",
	),
	"workspace start": h(
		"Launch the project supervisor and workspace orchestrator in tmux. In a completed workspace this starts or resumes a conversation-only Run; it does not reopen task execution. The current terminal view is not changed.",
		"workspace start --workspace ws_01",
	),
	"workspace attach": h(
		"Attach to the selected workspace's orchestrator tmux session.",
		"workspace attach --workspace ws_01",
	),
	"workspace pause": h(
		"Pause workflow delegation. Use --interrupt to stop active sessions while preserving their local work.",
		"workspace pause --interrupt",
	),
	"workspace resume": h(
		"Resume delegation for a paused workspace. It does not reactivate a completed workspace; use workspace reopen with a reason and exact revision for authorized new work.",
		"workspace resume",
	),
	"workspace complete": h(
		"Complete an intentionally manual workspace after the user explicitly confirms it. The workspace must have no active Runs, services or non-accepted tasks; a workspace with a selected workflow uses release confirmation instead.",
		"workspace complete --reason \"Analysis delivered\" --user-confirmed",
	),
	"workspace reopen": h(
		"Reopen only a completed workspace for explicitly authorized follow-up work. The exact current revision and a non-empty reason are required; an agent actor must also pass --user-confirmed. The prior WORKSPACE.md is preserved under history/reopen_ID/.",
		"workspace reopen --reason \"User requested follow-up fixes\" --expected-revision 12 --operation-key reopen-1",
	),
	"workspace archive": h(
		"Archive a completed workspace so it can no longer receive normal delegation operations. A workflow workspace requires a confirmed release; a completed manual workspace does not.",
		"workspace archive",
	),
	"workspace clean": h(
		"Remove archived worktrees after checking preservation. Use --dry-run to inspect the removal plan first.",
		"workspace clean --dry-run\nworkspace clean --backup",
	),
	"workspace reconcile": h(
		"Reconcile durable session records with the tmux panes that currently exist.",
		"workspace reconcile",
	),
	"workspace menu": h(
		"Show workflow actions and pending decisions available to the orchestrator or user.",
		"workspace menu",
	),
}

// flagHelpSpecs improves the most frequently misunderstood options while
// retaining all existing parsing and validation behavior.
var flagHelpSpecs = map[string]map[string]string{
	"workspace": {
		"project":         "Project path; defaults to WORKSPACE_PROJECT_DIR or the current directory.",
		"workspace":       "Workspace ID; defaults to WORKSPACE_ID or the workspace inferred from the current directory.",
		"tmux-socket":     "Optional isolated tmux server name.",
		"operation-key":   "Idempotency key for a mutation; reusing it with a different payload is rejected.",
		"json":            "Print machine-readable {ok,data} or {ok:false,error} JSON.",
		"short":           "Print a compact human-readable summary instead of the full result.",
		"non-interactive": "Never prompt; return a decision-required error when input is missing.",
	},
	"workspace list":          {"map": "Alias for --short; print a compact name-to-ID map."},
	"workspace skill install": {"client": "Client discovery target: codex, claude or opencode."},
	"workspace create": {
		"title":       "Workspace title shown in selectors and compact output.",
		"input-file":  "File containing the saved issue or task description.",
		"issue":       "Issue URL; fetch it from the configured tracker unless intent or --input-file is supplied.",
		"workflow":    "Configured workflow name; omit it to choose a workflow later.",
		"no-workflow": "Create an active workspace with no workflow for manual orchestration; cannot be combined with --workflow.",
		"base":        "Base Git revision to freeze for the workspace.",
		"from-issue":  "Existing durable Issue ID; mutually exclusive with intent, --input-file and --issue.",
	},
	"workspace issue create": {
		"title":      "Issue title; tracker title is used when omitted.",
		"input-file": "File containing a manual Issue body.",
		"issue":      "HTTP(S) source URL; tracker access is read-only and skipped when a body is supplied.",
	},
	"workspace issue refresh": {
		"expected-revision": "Exact current Issue revision required before refresh.",
	},
	"workspace issue update": {
		"status":            "Local status: open, deferred, or closed.",
		"reason":            "Required for deferred/closed; recorded when reopening to open if supplied.",
		"expected-revision": "Exact current Issue revision required before update.",
	},
	"workspace issue dispatch": {
		"workflow":    "Configured workflow; mutually exclusive with --no-workflow.",
		"no-workflow": "Create an active manual Workspace.",
		"base":        "Base Git revision to freeze for the Workspace.",
		"title":       "Workspace title override; defaults to the Issue title.",
		"start":       "Explicitly start the linked Workspace Orchestrator after creation.",
	},
	"workspace dispatcher start": {
		"profile": "Explicit Dispatcher profile override; otherwise use defaults.dispatcher_profile, then defaults.orchestrator_profile.",
	},
	"workspace workflow advance": {"to": "Expected next phase; fail if the workflow would advance from a different phase."},
	"workspace workflow migrate": {
		"reason":            "Why this workflow-template revision is required (required).",
		"expected-revision": "Current workspace revision required for this migration (required).",
	},
	"workspace input update": {
		"reason":            "Reason recorded for the input revision (required).",
		"expected-revision": "Current workspace revision required for this update (required).",
		"input-file":        "Revised issue description file.",
		"issue":             "Source URL for the revised input.",
	},
	"workspace agent create": {
		"role":              "Persona role: planner, implementer, integrator or tester (required).",
		"profile":           "Default model profile for sessions of this persona.",
		"prompt-template":   "Prompt template name to snapshot for this persona.",
		"instructions-file": "File containing delegated-scope and persona instructions.",
	},
	"workspace worktree create": {
		"base":    "Base revision; defaults to the frozen workspace commit.",
		"purpose": "Checkout purpose, such as planning or implementation.",
	},
	"workspace session start": {
		"agent":           "Agent ID or persona name to launch (required).",
		"worktree":        "Worktree ID or name for the session checkout (required for worker sessions).",
		"parent":          "Parent agent ID; defaults to the orchestrator.",
		"parent-session":  "Exact parent logical Session ID; captured automatically for an active delegating agent.",
		"profile":         "Override the persona's model profile.",
		"task":            "Task ID or name assigned to the session.",
		"prompt-template": "Override the snapshotted prompt template.",
		"read-only":       "Share the checkout for analysis without repository write rights.",
	},
	"workspace session bind-thread": {"thread-id": "Native client conversation or session ID."},
	"workspace task create":         {"spec-file": "YAML or Markdown frontmatter task specification file (required)."},
	"workspace task retry":          {"reason": "Reason recorded for starting the new attempt."},
	"workspace message send": {
		"body-file":  "File containing the message body (required).",
		"to":         "Compatibility recipient Agent ID or persona name; accepted only when exactly one eligible Session exists.",
		"to-session": "Exact logical recipient Session ID (preferred).",
		"kind":       "Message kind: note, question, answer or status.",
		"reply-to":   "Original message ID when this is a reply.",
	},
	"workspace inbox list": {"agent": "Explicit agent-wide historical inbox owner.", "session": "Exact logical Session to inspect.", "all": "Include acknowledged messages."},
	"workspace inbox read": {"session": "Exact logical Session to inspect."},
	"workspace inbox ack":  {"session": "Exact logical Session to inspect and acknowledge."},
	"workspace inbox wait": {"timeout": "Maximum wait in seconds.", "agent": "Explicit agent-wide historical inbox owner.", "session": "Exact logical Session to await."},
	"workspace handoff submit": {
		"to":           "Compatibility parent or orchestrator Agent ID; accepted only when exactly one eligible Session exists.",
		"to-session":   "Exact parent or orchestrator logical Session ID (preferred).",
		"task":         "Task ID or name for the result.",
		"session":      "Producing session; inferred for agents when omitted.",
		"outcome":      "Result outcome: succeeded, blocked or failed.",
		"summary-file": "Summary text file (required).",
		"artifact":     "Explicit artifact path; repeat the flag or provide comma-separated paths.",
		"checks-file":  "YAML/JSON array of command, exit_code and evidence filename.",
		"check":        "Captured check receipt ID; repeat for multiple checks.",
		"risk":         "Known risk; repeat for multiple risks.",
	},
	"workspace handoff accept":      {"reason-file": "Review feedback file."},
	"workspace handoff reject":      {"reason-file": "Review feedback file; required when rejecting."},
	"workspace integration prepare": {"tasks": "Accepted task IDs; repeat or comma-separate. Defaults to all implementation tasks.", "base": "Target base revision."},
	"workspace decision answer": {
		"answer":            "Selected option or answer text (required).",
		"reason":            "User's explanation for the answer.",
		"environment":       "Live-testing environment or URL (required when answer is run).",
		"expected-revision": "Revision of the pending decision (required).",
		"user-confirmed":    "Attest that this answer was explicitly supplied by the user.",
	},
	"workspace release confirm":        {"reference": "Release or deployment reference (required).", "user-confirmed": "Attest explicit user release confirmation when running from an agent session."},
	"workspace complete":               {"reason": "Reason recorded for the completion.", "user-confirmed": "Attest the explicit user completion request when running from an agent session.", "expected-revision": "Required current workspace revision."},
	"workspace state update":           {"patch-file": "YAML/JSON patch for title, body, status or phase (required).", "expected-revision": "Required current workspace revision (required)."},
	"workspace state edit":             {"file": "Import an edited WORKSPACE.md instead of opening EDITOR.", "expected-revision": "Required revision when using --file."},
	"workspace change-request prepare": {"worktree": "Worktree to diff; defaults to integration.", "target": "Target branch.", "title": "Change-request title.", "body-file": "Change-request description file."},
	"workspace change-request publish": {"user-confirmed": "Attest that the user approved the prepared diff and description (required in agent sessions)."},
	"workspace change-request skip":    {"reason": "User's reason or evidence (required).", "url": "Existing change-request URL, when linking instead of skipping.", "user-confirmed": "Attest the explicit user decision (required in agent sessions)."},
	"workspace change-request link":    {"reason": "User's reason or evidence.", "url": "Existing change-request URL (required).", "user-confirmed": "Attest the explicit user decision (required in agent sessions)."},
	"workspace change-request retry":   {"reason": "User's reason or evidence (required).", "url": "Existing change-request URL.", "user-confirmed": "Attest the explicit user decision (required in agent sessions)."},
	"workspace check run":              {"session": "Producing session; inferred for agents and must be task-bound."},
	"workspace service start":          {"worktree": "Assigned worktree for the service (required)."},
	"workspace pause":                  {"interrupt": "Stop active Runs while preserving logical Sessions and local work."},
	"workspace clean":                  {"dry-run": "Show the removal plan without changes.", "backup": "Preserve unpublished commits in verified Git bundles before removal."},
}

func configureHelp(root *cobra.Command) {
	root.SetUsageTemplate(helpUsageTemplate)

	for path, spec := range commandHelpSpecs {
		cmd := commandAtPath(root, path)
		if cmd == nil {
			continue
		}
		cmd.Long = spec.long
		cmd.Example = spec.example
		if len(spec.arguments) > 0 {
			if cmd.Annotations == nil {
				cmd.Annotations = map[string]string{}
			}
			cmd.Annotations["workspace.arguments"] = renderArguments(spec.arguments)
		}
	}

	for path, flags := range flagHelpSpecs {
		cmd := commandAtPath(root, path)
		if cmd == nil {
			continue
		}
		for name, usage := range flags {
			flag := cmd.Flags().Lookup(name)
			if flag == nil {
				flag = cmd.PersistentFlags().Lookup(name)
			}
			if flag != nil {
				flag.Usage = usage
			}
		}
	}
}

func renderArguments(arguments []helpArgument) string {
	width := 0
	displays := make([]string, len(arguments))
	for i, argument := range arguments {
		display := argument.name
		if !strings.HasPrefix(display, "--") {
			if argument.required {
				display = "<" + display + ">"
			} else {
				display = "[" + display + "]"
			}
		}
		displays[i] = display
		if len(display) > width {
			width = len(display)
		}
	}
	var b strings.Builder
	for i, argument := range arguments {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, displays[i], argument.description)
	}
	return strings.TrimRight(b.String(), "\n")
}

func commandAtPath(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Fields(path)
	if len(parts) == 0 || parts[0] != root.Name() {
		return nil
	}
	current := root
	for _, part := range parts[1:] {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func visibleCommandPaths(root *cobra.Command) []string {
	var paths []string
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if cmd != root && (!cmd.IsAvailableCommand() || cmd.Name() == "help") {
			return
		}
		if cmd == root || cmd.Name() != "help" {
			paths = append(paths, cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
	sort.Strings(paths)
	return paths
}

func missingHelpDocumentation(root *cobra.Command) []string {
	var missing []string
	for _, path := range visibleCommandPaths(root) {
		if _, ok := commandHelpSpecs[path]; !ok {
			missing = append(missing, path)
		}
	}
	return missing
}

// normalizeHelpArgs makes the convenient suffix form work for leaf commands
// as well as groups. A standalone -- ends parsing and therefore preserves a
// literal final "help" argument for commands that accept one. Internal
// runners are excluded so their private argument contract remains unchanged.
func normalizeHelpArgs(args []string) []string {
	if len(args) == 0 || args[len(args)-1] != "help" {
		return args
	}
	for _, arg := range args[:len(args)-1] {
		if arg == "--" || arg == "_session-exec" || arg == "_service-exec" || arg == "_dispatcher-exec" {
			return args
		}
	}
	out := append([]string(nil), args[:len(args)-1]...)
	return append(out, "--help")
}
