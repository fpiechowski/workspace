package core

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Service) ReconcileInterface(ctx context.Context, selector string) error {
	uiRuntime, ok := s.Runtime.(ManagedUIRuntime)
	reader, hasTopology := s.Runtime.(RuntimeTopologyReader)
	if !ok || !hasTopology {
		return nil
	}
	return withProjectLock(ctx, s.Root, func() error {
		dir, err := s.resolveReadableWorkspace(selector)
		if err != nil {
			return err
		}
		doc, err := loadDocument(dir)
		if err != nil {
			return err
		}
		record, exists, err := readUIRecord(dir, doc.State.ID, doc.State.ProjectID)
		if err != nil {
			return err
		}
		archived := doc.State.Status == "archived"
		if !exists && (archived || !hasOrchestratorHistory(doc)) {
			return nil
		}
		if !exists {
			record.UIID = ID("ui")
			record.Desired = true
		}
		if archived {
			record.Desired = false
			record.ExitRequested = false
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		topology, topologyErr := reader.ObserveTopology(callCtx, doc.State.ID)
		cancel()
		if topologyErr != nil {
			if exists {
				setUIBackoff(&record, topologyErr.Error(), nowUTC())
				if err := writeUIRecord(dir, record); err != nil {
					return errors.Join(topologyErr, err)
				}
			}
			return topologyErr
		}

		launchBase := UILaunch{ProjectRoot: s.Root, ProjectID: doc.State.ProjectID, WorkspaceID: doc.State.ID, WorkspaceDir: dir, Executable: s.Executable}
		if tmux, ok := s.Runtime.(Tmux); ok {
			launchBase.Socket = tmux.Socket
		}
		ownedPane, err := reconcileUIPanes(ctx, uiRuntime, launchBase, topology, record)
		if err != nil {
			setUIBackoff(&record, err.Error(), nowUTC())
			if writeErr := writeUIRecord(dir, record); writeErr != nil {
				return errors.Join(err, writeErr)
			}
			return err
		}

		if !record.Desired {
			if pane := ownedPane; pane != nil {
				if record.ExitRequested && !pane.Dead {
					if record.State != "disabled" || record.LastError != "waiting for the managed interface to exit" {
						record.State = "disabled"
						record.LastError = "waiting for the managed interface to exit"
						record.UpdatedAt = nowUTC()
						if err := writeUIRecord(dir, record); err != nil {
							return err
						}
					}
					return nil
				}
			}
			if record.State == "disabled" && record.PaneID == "" && record.WindowID == "" && !record.ExitRequested {
				return nil
			}
			record.State = "disabled"
			record.PaneID, record.WindowID, record.AnchorRunID = "", "", ""
			record.LastError = ""
			record.NextRetryAt = nil
			record.ExitRequested = false
			record.UpdatedAt = nowUTC()
			return writeUIRecord(dir, record)
		}
		if archived {
			record.State = "disabled"
			record.PaneID, record.WindowID = "", ""
			record.LastError = ""
			record.UpdatedAt = nowUTC()
			return writeUIRecord(dir, record)
		}
		if !topology.SessionExists {
			return setUIWaiting(dir, &record, "workspace tmux session does not exist", true)
		}
		if !hasOrchestratorHistory(doc) {
			return setUIWaiting(dir, &record, "no orchestrator session has been recorded", true)
		}
		anchorCtx, anchorCancel := context.WithTimeout(ctx, 2*time.Second)
		windowID, anchorRunID, err := orchestratorAnchor(anchorCtx, s, doc, topology)
		anchorCancel()
		if err != nil || windowID == "" {
			if err == nil {
				err = fail("ui_waiting_for_runtime", "no verified orchestrator window is available")
			}
			return setUIWaiting(dir, &record, err.Error(), false)
		}
		oldPaneID, oldWindowID, oldAnchorRunID := record.PaneID, record.WindowID, record.AnchorRunID
		record.AnchorRunID = anchorRunID
		if pane := ownedPane; pane != nil {
			launch := launchBase
			launch.UIID, launch.Generation, launch.Token = record.UIID, record.Generation, record.LaunchToken
			launch.WindowID = windowID
			if pane.Dead {
				stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
				err := uiRuntime.StopUI(stopCtx, launch, pane.ID)
				stopCancel()
				if err != nil {
					setUIBackoff(&record, err.Error(), nowUTC())
					_ = writeUIRecord(dir, record)
					return err
				}
				setUIBackoff(&record, "managed interface process exited", nowUTC())
				record.PaneID, record.WindowID = "", ""
				return writeUIRecord(dir, record)
			}
			if !uiPaneMetadataMatches(topology, pane.ID, record) {
				repairCtx, repairCancel := context.WithTimeout(ctx, 2*time.Second)
				repaired, repairErr := uiRuntime.LaunchUI(repairCtx, launch)
				repairCancel()
				if repairErr != nil || repaired.ID != pane.ID {
					if repairErr == nil {
						repairErr = fail("ui_pane_conflict", "metadata repair returned a different managed interface pane")
					}
					setUIBackoff(&record, repairErr.Error(), nowUTC())
					_ = writeUIRecord(dir, record)
					return repairErr
				}
				pane = &repaired
			}
			record.PaneID, record.WindowID = pane.ID, pane.WindowID
			if record.State == "running" {
				changed := record.LastError != "" || record.NextRetryAt != nil
				record.LastError = ""
				if record.StartedAt != nil && time.Since(*record.StartedAt) >= 30*time.Second && record.Failures != 0 {
					record.Failures = 0
					record.NextRetryAt = nil
					changed = true
				}
				changed = changed || oldPaneID != pane.ID || oldWindowID != pane.WindowID || oldAnchorRunID != anchorRunID
				if !changed {
					return nil
				}
				record.UpdatedAt = nowUTC()
				return writeUIRecord(dir, record)
			}
			if record.State == "starting" && record.UpdatedAt.Add(10*time.Second).After(nowUTC()) {
				if record.PaneID == pane.ID && record.WindowID == pane.WindowID {
					return nil
				}
				record.PaneID, record.WindowID = pane.ID, pane.WindowID
				record.UpdatedAt = nowUTC()
				return writeUIRecord(dir, record)
			}
			if record.State == "backoff" && record.NextRetryAt != nil && nowUTC().Before(*record.NextRetryAt) {
				return nil
			}
			stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
			err := uiRuntime.StopUI(stopCtx, launch, pane.ID)
			stopCancel()
			if err != nil {
				setUIBackoff(&record, err.Error(), nowUTC())
				_ = writeUIRecord(dir, record)
				return err
			}
			setUIBackoff(&record, "managed interface did not claim its launch", nowUTC())
			_ = writeUIRecord(dir, record)
			return nil
		}
		if record.State == "running" {
			setUIBackoff(&record, "managed interface pane was lost", nowUTC())
			record.PaneID, record.WindowID = "", ""
			return writeUIRecord(dir, record)
		}
		if record.NextRetryAt != nil && nowUTC().Before(*record.NextRetryAt) {
			record.State = "backoff"
			return nil
		}
		if record.State == "starting" && record.UpdatedAt.Add(10*time.Second).After(nowUTC()) {
			return nil
		}
		if record.State == "starting" && record.Generation > 0 {
			setUIBackoff(&record, "launch result was not observed", nowUTC())
			_ = writeUIRecord(dir, record)
			return nil
		}

		record.Generation++
		if record.Generation < 1 {
			record.Generation = 1
		}
		record.LaunchToken = ID("uit")
		record.State = "starting"
		record.PaneID, record.WindowID = "", ""
		record.AnchorRunID = anchorRunID
		record.LastError = ""
		record.UpdatedAt = nowUTC()
		record.ExitRequested = false
		if err := writeUIRecord(dir, record); err != nil {
			return err
		}
		launch := launchBase
		launch.WindowID, launch.UIID, launch.Generation, launch.Token = windowID, record.UIID, record.Generation, record.LaunchToken
		launchCtx, launchCancel := context.WithTimeout(ctx, 2*time.Second)
		pane, err := uiRuntime.LaunchUI(launchCtx, launch)
		launchCancel()
		if err != nil {
			var ce *Error
			if errors.As(err, &ce) && (ce.Code == "ui_space" || ce.Code == "ui_waiting_for_runtime") {
				record.State, record.LastError = "waiting_for_runtime", err.Error()
				record.UpdatedAt = nowUTC()
				_ = writeUIRecord(dir, record)
				return err
			}
			if errors.As(err, &ce) && ce.Code == "launch_uncertain" {
				record.LastError = err.Error()
				record.UpdatedAt = nowUTC()
				_ = writeUIRecord(dir, record)
				return err
			}
			setUIBackoff(&record, err.Error(), nowUTC())
			_ = writeUIRecord(dir, record)
			return err
		}
		record.PaneID, record.WindowID = pane.ID, pane.WindowID
		record.State = "starting"
		record.UpdatedAt = nowUTC()
		return writeUIRecord(dir, record)
	})
}

func setUIWaiting(dir string, record *uiRecord, message string, clearPane bool) error {
	changed := record.State != "waiting_for_runtime" || record.LastError != message
	if clearPane && (record.PaneID != "" || record.WindowID != "") {
		record.PaneID, record.WindowID = "", ""
		changed = true
	}
	if !changed {
		return nil
	}
	record.State, record.LastError = "waiting_for_runtime", message
	record.UpdatedAt = nowUTC()
	return writeUIRecord(dir, *record)
}

func setUIBackoff(record *uiRecord, message string, now time.Time) {
	record.Failures++
	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 60 * time.Second}
	index := record.Failures - 1
	if index >= len(delays) {
		index = len(delays) - 1
	}
	retry := now.Add(delays[index])
	record.NextRetryAt = &retry
	record.State = "backoff"
	record.LastError = message
	record.UpdatedAt = now
}

func reconcileUIPanes(ctx context.Context, runtime ManagedUIRuntime, base UILaunch, topology TmuxTopology, record uiRecord) (*Pane, error) {
	if record.UIID == "" {
		return nil, nil
	}
	owned := make([]Pane, 0)
	for _, pane := range topology.Panes {
		uiID, generation, token, ok := uiPaneIdentity(pane, record.WorkspaceID)
		if ok && uiID == record.UIID {
			pane.UIID, pane.UIGeneration, pane.UIToken = uiID, generation, token
			owned = append(owned, pane)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].ID < owned[j].ID })

	var current *Pane
	if record.LaunchToken != "" && record.Generation > 0 {
		for i := range owned {
			pane := &owned[i]
			if pane.UIGeneration != record.Generation || pane.UIToken != record.LaunchToken {
				continue
			}
			if current == nil || pane.ID == record.PaneID || current.ID != record.PaneID && !pane.Dead && current.Dead {
				current = pane
			}
		}
	}

	keepCurrent := record.Desired || record.ExitRequested && current != nil && !current.Dead
	for i := range owned {
		pane := &owned[i]
		if keepCurrent && current != nil && current.ID == pane.ID {
			continue
		}
		launch := base
		launch.UIID, launch.Generation, launch.Token = pane.UIID, pane.UIGeneration, pane.UIToken
		stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := runtime.StopUI(stopCtx, launch, pane.ID)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	if !keepCurrent {
		return nil, nil
	}
	return current, nil
}

func uiPaneMetadataMatches(topology TmuxTopology, paneID string, record uiRecord) bool {
	for _, pane := range topology.Panes {
		if pane.ID == paneID {
			return pane.Kind == "tui" && pane.WorkspaceID == record.WorkspaceID && pane.UIID == record.UIID && pane.UIGeneration == record.Generation && pane.UIToken == record.LaunchToken
		}
	}
	return false
}

func uiPaneIdentity(pane Pane, workspaceID string) (string, int, string, bool) {
	if pane.Kind == "tui" && pane.WorkspaceID == workspaceID && pane.UIID != "" && pane.UIGeneration > 0 && pane.UIToken != "" {
		return pane.UIID, pane.UIGeneration, pane.UIToken, true
	}
	if pane.SessionName != TmuxName(workspaceID) || pane.StartCommand == "" {
		return "", 0, "", false
	}
	fields := strings.Fields(pane.StartCommand)
	for i, field := range fields {
		if strings.Trim(field, "'\"") != "_tui-exec" || i+3 >= len(fields) {
			continue
		}
		uiID := strings.Trim(fields[i+1], "'\"")
		generation, err := strconv.Atoi(strings.Trim(fields[i+2], "'\""))
		token := strings.Trim(fields[i+3], "'\"")
		if err == nil && uiID != "" && generation > 0 && token != "" {
			return uiID, generation, token, true
		}
	}
	return "", 0, "", false
}

func orchestratorAnchor(ctx context.Context, service *Service, doc *Document, topology TmuxTopology) (string, string, error) {
	type candidate struct {
		session Session
		run     Run
		active  bool
	}
	var candidates []candidate
	for _, session := range doc.Registry.Sessions {
		if session.AgentID != doc.State.OrchestratorAgentID {
			continue
		}
		runID := session.CurrentRunID
		active := runID != ""
		if runID == "" {
			runID = session.LastRunID
		}
		if runID == "" {
			continue
		}
		run, err := findRun(doc, runID)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{session: session, run: *run, active: active && run.Active()})
	}
	if len(candidates) == 0 {
		return "", "", nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].active != candidates[j].active {
			return candidates[i].active
		}
		return candidates[i].run.CreatedAt.After(candidates[j].run.CreatedAt)
	})
	preferred := candidates[0]
	if preferred.run.WindowID != "" {
		for i := range topology.Windows {
			window := &topology.Windows[i]
			if window.ID != preferred.run.WindowID {
				continue
			}
			if window.Kind == "orchestrator" {
				return window.ID, preferred.run.ID, nil
			}
			if window.Kind != "" {
				return "", preferred.run.ID, fail("ui_waiting_for_runtime", "recorded orchestrator window has conflicting ownership")
			}
			verified := false
			for _, pane := range topology.Panes {
				if pane.WindowID == window.ID && paneOwns(pane, preferred.session.ID, preferred.run.ID) && pane.WorkspaceID == doc.State.ID && pane.Kind != "tui" && pane.Kind != "service" {
					verified = true
					break
				}
			}
			if verified {
				if tmux, ok := service.Runtime.(Tmux); ok {
					launch := Launch{ProjectID: doc.State.ProjectID, WorkspaceID: doc.State.ID}
					if err := tmux.setWindowMetadata(ctx, window.ID, launch, "orchestrator"); err != nil {
						return "", preferred.run.ID, err
					}
				}
				return window.ID, preferred.run.ID, nil
			}
		}
	}
	var found *TmuxWindow
	for i := range topology.Windows {
		if topology.Windows[i].Kind != "orchestrator" {
			continue
		}
		if found != nil {
			return "", preferred.run.ID, fail("ui_waiting_for_runtime", "multiple orchestrator windows are available")
		}
		found = &topology.Windows[i]
	}
	if found == nil {
		return "", preferred.run.ID, nil
	}
	return found.ID, preferred.run.ID, nil
}

func uiRecordDigest(record uiRecord) string {
	b, _ := json.Marshal(record)
	return payloadDigest(string(b))
}
