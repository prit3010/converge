package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	agentRunSourceDefault = "agent_complete"
	agentRunStatusRunning = "running"
)

func (d *DB) ReserveAgentRun(run AgentRun) (bool, *AgentRun, error) {
	runID := strings.TrimSpace(run.RunID)
	if runID == "" {
		return false, nil, fmt.Errorf("run_id cannot be empty")
	}
	agent := strings.TrimSpace(run.Agent)
	if agent == "" {
		return false, nil, fmt.Errorf("agent cannot be empty")
	}
	source := strings.TrimSpace(run.Source)
	if source == "" {
		source = agentRunSourceDefault
	}
	message := strings.TrimSpace(run.Message)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	res, err := d.sql.Exec(`
INSERT INTO agent_runs (run_id, agent, message, tags, source, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO NOTHING
`, runID, agent, message, run.Tags, source, agentRunStatusRunning, now, now)
	if err != nil {
		return false, nil, fmt.Errorf("reserve agent run %s: %w", runID, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, nil, fmt.Errorf("reserve agent run rows affected %s: %w", runID, err)
	}
	if affected == 1 {
		created := AgentRun{
			RunID:     runID,
			Agent:     agent,
			Message:   message,
			Tags:      run.Tags,
			Source:    source,
			Status:    agentRunStatusRunning,
			CreatedAt: now,
			UpdatedAt: now,
		}
		return true, &created, nil
	}

	existing, err := d.GetAgentRun(runID)
	if err != nil {
		return false, nil, err
	}
	return false, existing, nil
}

func (d *DB) FinalizeAgentRun(runID, status string, cellID, branch *string, source string, runErr *string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("run_id cannot be empty")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return fmt.Errorf("status cannot be empty")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = agentRunSourceDefault
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := d.sql.Exec(`
UPDATE agent_runs
SET status = ?, cell_id = ?, branch = ?, source = ?, error = ?, updated_at = ?
WHERE run_id = ?
`, status, cellID, branch, source, runErr, updatedAt, runID)
	if err != nil {
		return fmt.Errorf("finalize agent run %s: %w", runID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("finalize agent run rows affected %s: %w", runID, err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) GetAgentRun(runID string) (*AgentRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id cannot be empty")
	}
	row := d.sql.QueryRow(`
SELECT run_id, agent, message, tags, source, status, branch, cell_id, error, created_at, updated_at
FROM agent_runs
WHERE run_id = ?
`, runID)
	var run AgentRun
	if err := row.Scan(
		&run.RunID,
		&run.Agent,
		&run.Message,
		&run.Tags,
		&run.Source,
		&run.Status,
		&run.Branch,
		&run.CellID,
		&run.Error,
		&run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get agent run %s: %w", runID, err)
	}
	return &run, nil
}
