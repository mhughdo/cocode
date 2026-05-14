package followup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hughdo/cocode/services/cocoded/internal/contextbundle"
	"github.com/hughdo/cocode/services/cocoded/internal/db/dbgen"
)

const (
	QuickActionAskCounterEvidence = "ask_counter_evidence"
	QuickActionAccept             = "accept"
	QuickActionDismiss            = "dismiss"
	QuickActionCopy               = "copy"

	defaultCounterEvidenceQuestion = "Look for real counter-evidence for this finding. Only call something counter-evidence if it refutes the claim; otherwise label it as related context or a test signal. Cite the strongest evidence refs and say if the scoped context is insufficient."
)

type QuickActionParams struct {
	FindingID       string
	ReviewSessionID string
	Action          string
	Reason          string
	AgentConfigID   string
	ContextPolicy   json.RawMessage
}

type QuickActionResult struct {
	Action           string
	View             ThreadView
	Finding          dbgen.Finding
	Decision         dbgen.HumanDecision
	Message          dbgen.FindingThreadMessage
	AssistantMessage dbgen.FindingThreadMessage
	AgentRun         dbgen.AgentRun
	ContextBundle    contextbundle.Bundle
}

func (s Service) RunQuickAction(ctx context.Context, params QuickActionParams) (QuickActionResult, error) {
	action, err := normalizeQuickAction(params.Action)
	if err != nil {
		return QuickActionResult{}, err
	}
	switch action {
	case QuickActionAskCounterEvidence:
		return s.askCounterEvidence(ctx, params)
	case QuickActionAccept, QuickActionDismiss, QuickActionCopy:
		return s.recordQuickDecision(ctx, params, action)
	default:
		return QuickActionResult{}, fmt.Errorf("%w: %s", ErrInvalidQuickAction, action)
	}
}

func (s Service) askCounterEvidence(ctx context.Context, params QuickActionParams) (QuickActionResult, error) {
	answer, err := s.AskQuestion(ctx, AskQuestionParams{
		FindingID:       params.FindingID,
		ReviewSessionID: params.ReviewSessionID,
		Question:        defaultCounterEvidenceQuestion,
		AgentConfigID:   params.AgentConfigID,
		ContextPolicy:   params.ContextPolicy,
	})
	if err != nil {
		return QuickActionResult{}, err
	}
	return QuickActionResult{
		Action:           QuickActionAskCounterEvidence,
		View:             answer.View,
		Finding:          answer.View.Finding,
		Message:          answer.UserMessage,
		AssistantMessage: answer.AssistantMessage,
		AgentRun:         answer.AgentRun,
		ContextBundle:    answer.ContextBundle,
	}, nil
}

func (s Service) recordQuickDecision(ctx context.Context, params QuickActionParams, action string) (QuickActionResult, error) {
	if s.Queries == nil {
		return QuickActionResult{}, ErrServiceNotConfigured
	}
	if s.Database == nil {
		return QuickActionResult{}, fmt.Errorf("%w: database is required for quick actions", ErrServiceNotConfigured)
	}
	decision, err := quickActionDecision(action, params.Reason)
	if err != nil {
		return QuickActionResult{}, err
	}
	view, err := s.EnsureThread(ctx, EnsureThreadParams{
		FindingID:       params.FindingID,
		ReviewSessionID: params.ReviewSessionID,
	})
	if err != nil {
		return QuickActionResult{}, err
	}
	tx, err := s.Database.BeginTx(ctx, nil)
	if err != nil {
		return QuickActionResult{}, fmt.Errorf("begin follow-up quick action: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txQueries := s.Queries.WithTx(tx)
	now := s.now().Format(time.RFC3339Nano)
	updated, err := txQueries.UpdateFindingDecisionStatus(ctx, dbgen.UpdateFindingDecisionStatusParams{
		ID:             view.Finding.ID,
		DecisionStatus: decision,
		UpdatedAt:      now,
	})
	if err != nil {
		return QuickActionResult{}, fmt.Errorf("update quick action decision: %w", err)
	}
	metadata, err := quickActionDecisionMetadata(action, view.Thread.ID)
	if err != nil {
		return QuickActionResult{}, err
	}
	storedDecision, err := txQueries.CreateHumanDecision(ctx, dbgen.CreateHumanDecisionParams{
		ID:              s.newID("human_decision_"),
		FindingID:       updated.ID,
		ReviewSessionID: updated.ReviewSessionID,
		Decision:        decision,
		Reason:          nullableString(params.Reason),
		MetadataJson:    string(metadata),
		CreatedAt:       now,
	})
	if err != nil {
		return QuickActionResult{}, fmt.Errorf("store quick action decision: %w", err)
	}
	txService := Service{
		Database: s.Database,
		Queries:  txQueries,
		Now:      s.Now,
		NewID:    s.NewID,
	}
	message, err := txService.AppendMessage(ctx, AppendMessageParams{
		ThreadID: view.Thread.ID,
		Role:     MessageRoleSystem,
		Content:  quickActionMessage(action, strings.TrimSpace(params.Reason)),
	})
	if err != nil {
		return QuickActionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuickActionResult{}, fmt.Errorf("commit follow-up quick action: %w", err)
	}
	committed = true

	reloaded, err := s.LoadThread(ctx, view.Thread.ID)
	if err != nil {
		return QuickActionResult{}, err
	}
	reloaded.Finding = updated
	return QuickActionResult{
		Action:   action,
		View:     reloaded,
		Finding:  updated,
		Decision: storedDecision,
		Message:  message,
	}, nil
}

func normalizeQuickAction(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "ask_counter_evidence", "counter_evidence", "counter-evidence", "ask-counter-evidence":
		return QuickActionAskCounterEvidence, nil
	case "accept", "accepted":
		return QuickActionAccept, nil
	case "dismiss", "dismissed":
		return QuickActionDismiss, nil
	case "copy", "copied":
		return QuickActionCopy, nil
	default:
		return "", fmt.Errorf("%w: action is required", ErrInvalidQuickAction)
	}
}

func quickActionDecision(action string, reason string) (string, error) {
	switch action {
	case QuickActionAccept:
		return "accepted", nil
	case QuickActionDismiss:
		if strings.TrimSpace(reason) == "" {
			return "", fmt.Errorf("%w: reason is required when dismissing a finding", ErrInvalidQuickAction)
		}
		return "dismissed", nil
	case QuickActionCopy:
		return "copied", nil
	default:
		return "", fmt.Errorf("%w: action does not record a decision", ErrInvalidQuickAction)
	}
}

func quickActionDecisionMetadata(action string, threadID string) (json.RawMessage, error) {
	metadata, err := json.Marshal(map[string]string{
		"source":    "follow_up_quick_action",
		"action":    action,
		"thread_id": threadID,
	})
	if err != nil {
		return nil, fmt.Errorf("encode quick action metadata: %w", err)
	}
	return metadata, nil
}

func quickActionMessage(action string, reason string) string {
	switch action {
	case QuickActionAccept:
		if reason != "" {
			return "Accepted finding. Reason: " + reason
		}
		return "Accepted finding."
	case QuickActionDismiss:
		return "Dismissed finding. Reason: " + reason
	case QuickActionCopy:
		if reason != "" {
			return "Marked finding as copied. Note: " + reason
		}
		return "Marked finding as copied."
	default:
		return "Updated finding from follow-up."
	}
}
