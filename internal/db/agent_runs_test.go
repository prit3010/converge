package db

import "testing"

func TestReserveAgentRunAndDuplicateLookup(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	tags := "auto,codex"
	reserved, created, err := d.ReserveAgentRun(AgentRun{
		RunID:   "run-1",
		Agent:   "codex",
		Message: "done",
		Tags:    &tags,
	})
	if err != nil {
		t.Fatalf("reserve first run: %v", err)
	}
	if !reserved {
		t.Fatalf("expected first reserve to create row")
	}
	if created == nil {
		t.Fatalf("expected created run details")
	}
	if created.Status != agentRunStatusRunning {
		t.Fatalf("expected running status, got %s", created.Status)
	}

	reserved, existing, err := d.ReserveAgentRun(AgentRun{
		RunID:   "run-1",
		Agent:   "codex",
		Message: "done again",
	})
	if err != nil {
		t.Fatalf("reserve duplicate run: %v", err)
	}
	if reserved {
		t.Fatalf("expected duplicate reserve to be rejected")
	}
	if existing == nil {
		t.Fatalf("expected existing run for duplicate")
	}
	if existing.RunID != "run-1" {
		t.Fatalf("unexpected run id: %s", existing.RunID)
	}
	if existing.Tags == nil || *existing.Tags != tags {
		t.Fatalf("expected tags to persist as %s, got %v", tags, existing.Tags)
	}
}

func TestFinalizeAgentRunPersistsResult(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	if _, _, err := d.ReserveAgentRun(AgentRun{
		RunID:   "run-2",
		Agent:   "claude",
		Message: "final response",
		Source:  "agent_complete",
	}); err != nil {
		t.Fatalf("reserve run: %v", err)
	}

	cellID := "c_000007"
	branch := "main"
	if err := d.FinalizeAgentRun("run-2", "created", &cellID, &branch, "agent_complete", nil); err != nil {
		t.Fatalf("finalize run: %v", err)
	}

	got, err := d.GetAgentRun("run-2")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "created" {
		t.Fatalf("expected created status, got %s", got.Status)
	}
	if got.CellID == nil || *got.CellID != cellID {
		t.Fatalf("expected cell_id %s, got %v", cellID, got.CellID)
	}
	if got.Branch == nil || *got.Branch != branch {
		t.Fatalf("expected branch %s, got %v", branch, got.Branch)
	}
}

func TestFinalizeAgentRunMissingRow(t *testing.T) {
	d := openTestDB(t)
	defer d.Close()

	if err := d.FinalizeAgentRun("missing-run", "failed", nil, nil, "agent_complete", strPtr("boom")); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func strPtr(s string) *string {
	return &s
}
