package core

import "fmt"

func CellID(sequence int) string {
	return fmt.Sprintf("c_%06d", sequence)
}

type SnapOptions struct {
	Message string
	Tags    string
	Agent   string
	Source  string
	RunEval bool
}

type WorkingTreeDelta struct {
	Modified int
	Added    int
	Removed  int
}

const (
	AgentCompletionStatusCreated   = "created"
	AgentCompletionStatusNoChange  = "no_change"
	AgentCompletionStatusDuplicate = "duplicate"
	AgentCompletionStatusFailed    = "failed"
)

type AgentCompletionOptions struct {
	RunID   string
	Agent   string
	Message string
	Tags    string
	Source  string
	RunEval bool
}

type AgentCompletionResult struct {
	Status string  `json:"status"`
	RunID  string  `json:"run_id"`
	CellID *string `json:"cell_id,omitempty"`
	Branch string  `json:"branch,omitempty"`
	Source string  `json:"source,omitempty"`
	Error  string  `json:"error,omitempty"`
}
