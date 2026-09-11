package core

import "testing"

func TestDependentChangeRequestsRequireCurrentPredecessors(t *testing.T) {
	d := &Document{}
	d.State.Tasks = []Task{{ID: "plan", TaskSpec: TaskSpec{Role: "planner"}}, {ID: "api", AcceptedHandoff: "h-api", TaskSpec: TaskSpec{Role: "implementer"}}, {ID: "ui", TaskSpec: TaskSpec{Role: "implementer", DependsOn: []string{"plan", "api"}}}}
	d.Registry.Handoffs = []Handoff{{ID: "h-api", HeadCommit: "accepted-api-head"}}
	_, err := changeRequestDependencies(d, &d.State.Tasks[2])
	expectCode(t, err, "change_request_dependency")
	d.State.ChangeRequests = []ChangeRequest{{ID: "cr-old", TaskID: "api", HeadCommit: "old-head", State: "open"}}
	_, err = changeRequestDependencies(d, &d.State.Tasks[2])
	expectCode(t, err, "change_request_dependency")
	d.State.ChangeRequests = append(d.State.ChangeRequests, ChangeRequest{ID: "cr-api", TaskID: "api", HeadCommit: "accepted-api-head", State: "prepared"})
	deps, err := changeRequestDependencies(d, &d.State.Tasks[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0] != "cr-api" {
		t.Fatal("wrong dependency order", deps)
	}
	d.State.ChangeRequests[1].State = "outdated"
	_, err = changeRequestDependencies(d, &d.State.Tasks[2])
	expectCode(t, err, "change_request_dependency")
}
