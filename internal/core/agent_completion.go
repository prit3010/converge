package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/prittamravi/converge/internal/db"
)

const defaultAgentCompletionSource = "agent_complete"

func (s *Service) HandleAgentCompletion(ctx context.Context, opts AgentCompletionOptions) (AgentCompletionResult, error) {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return AgentCompletionResult{}, fmt.Errorf("run-id is required")
	}
	agent := strings.TrimSpace(opts.Agent)
	if agent == "" {
		return AgentCompletionResult{}, fmt.Errorf("agent is required")
	}
	message := strings.TrimSpace(opts.Message)
	if message == "" {
		return AgentCompletionResult{}, fmt.Errorf("message is required")
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = defaultAgentCompletionSource
	}

	tags := strings.TrimSpace(opts.Tags)
	var tagsPtr *string
	if tags != "" {
		tagsPtr = &tags
	}

	reserved, existingRun, err := s.DB.ReserveAgentRun(db.AgentRun{
		RunID:   runID,
		Agent:   agent,
		Message: message,
		Tags:    tagsPtr,
		Source:  source,
	})
	if err != nil {
		return AgentCompletionResult{}, err
	}
	if !reserved {
		return resultFromDuplicate(existingRun), nil
	}

	cell, created, createErr := s.CreateCellIfChanged(ctx, SnapOptions{
		Message: message,
		Tags:    tags,
		Agent:   agent,
		Source:  source,
		RunEval: opts.RunEval,
	})
	if createErr != nil {
		errText := createErr.Error()
		if finalizeErr := s.DB.FinalizeAgentRun(runID, AgentCompletionStatusFailed, nil, nil, source, &errText); finalizeErr != nil {
			return AgentCompletionResult{}, fmt.Errorf("finalize failed run: %w", finalizeErr)
		}
		return AgentCompletionResult{
			Status: AgentCompletionStatusFailed,
			RunID:  runID,
			Source: source,
			Error:  errText,
		}, createErr
	}

	if !created {
		branch, branchErr := s.ActiveBranch()
		if branchErr != nil {
			errText := branchErr.Error()
			if finalizeErr := s.DB.FinalizeAgentRun(runID, AgentCompletionStatusFailed, nil, nil, source, &errText); finalizeErr != nil {
				return AgentCompletionResult{}, fmt.Errorf("finalize failed run after branch error: %w", finalizeErr)
			}
			return AgentCompletionResult{
				Status: AgentCompletionStatusFailed,
				RunID:  runID,
				Source: source,
				Error:  errText,
			}, branchErr
		}
		branchCopy := branch
		if finalizeErr := s.DB.FinalizeAgentRun(runID, AgentCompletionStatusNoChange, nil, &branchCopy, source, nil); finalizeErr != nil {
			return AgentCompletionResult{}, fmt.Errorf("finalize no-change run: %w", finalizeErr)
		}
		return AgentCompletionResult{
			Status: AgentCompletionStatusNoChange,
			RunID:  runID,
			Branch: branch,
			Source: source,
		}, nil
	}

	if cell == nil {
		errText := "created cell missing from create flow"
		if finalizeErr := s.DB.FinalizeAgentRun(runID, AgentCompletionStatusFailed, nil, nil, source, &errText); finalizeErr != nil {
			return AgentCompletionResult{}, fmt.Errorf("finalize failed run after nil cell: %w", finalizeErr)
		}
		return AgentCompletionResult{
			Status: AgentCompletionStatusFailed,
			RunID:  runID,
			Source: source,
			Error:  errText,
		}, fmt.Errorf(errText)
	}

	cellID := cell.ID
	branch := strings.TrimSpace(cell.Branch)
	if finalizeErr := s.DB.FinalizeAgentRun(runID, AgentCompletionStatusCreated, &cellID, &branch, source, nil); finalizeErr != nil {
		return AgentCompletionResult{}, fmt.Errorf("finalize created run: %w", finalizeErr)
	}

	return AgentCompletionResult{
		Status: AgentCompletionStatusCreated,
		RunID:  runID,
		CellID: &cellID,
		Branch: branch,
		Source: source,
	}, nil
}

func resultFromDuplicate(run *db.AgentRun) AgentCompletionResult {
	if run == nil {
		return AgentCompletionResult{Status: AgentCompletionStatusDuplicate}
	}
	result := AgentCompletionResult{
		Status: AgentCompletionStatusDuplicate,
		RunID:  run.RunID,
		Source: run.Source,
	}
	if run.CellID != nil && strings.TrimSpace(*run.CellID) != "" {
		cellID := *run.CellID
		result.CellID = &cellID
	}
	if run.Branch != nil {
		result.Branch = strings.TrimSpace(*run.Branch)
	}
	if run.Error != nil {
		result.Error = strings.TrimSpace(*run.Error)
	}
	return result
}
