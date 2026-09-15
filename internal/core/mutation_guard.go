package core

// MutationGuard fences an interactive intent against the state shown in its
// confirmation form. Zero values are ignored for fields not used by an action.
type MutationGuard struct {
	ExpectedRevision   int      `json:"expected_revision,omitempty"`
	ExpectedRunID      string   `json:"expected_run_id,omitempty"`
	ExpectedAttempt    int      `json:"expected_attempt,omitempty"`
	ExpectedRunIDs     []string `json:"expected_run_ids,omitempty"`
	ExpectedServiceIDs []string `json:"expected_service_ids,omitempty"`
}

func (guard MutationGuard) empty() bool {
	return guard.ExpectedRevision == 0 && guard.ExpectedRunID == "" && guard.ExpectedAttempt == 0 && guard.ExpectedRunIDs == nil && guard.ExpectedServiceIDs == nil
}
