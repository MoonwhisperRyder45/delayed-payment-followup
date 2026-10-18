package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type PaymentEvent struct {
	PaymentID       string    `json:"payment_id"`
	OccurredAt      time.Time `json:"occurred_at"`
	NotBefore       time.Time `json:"not_before"`
	RiskLevel       string    `json:"risk_level"`
	Action          string    `json:"action"`
	NotificationRef string    `json:"notification_ref"`
}

type FollowupDecision struct {
	Due        bool   `json:"due"`
	Action     string `json:"action"`
	AuditEntry string `json:"audit_entry"`
}

func NewPaymentEvent(paymentID, riskLevel, notificationRef string, occurredAt time.Time, delay time.Duration) PaymentEvent {
	action := "send-payment-reminder"
	if riskLevel == "high" {
		action = "request-manual-review"
	}
	return PaymentEvent{
		PaymentID:       paymentID,
		OccurredAt:      occurredAt.UTC(),
		NotBefore:       occurredAt.Add(delay).UTC(),
		RiskLevel:       riskLevel,
		Action:          action,
		NotificationRef: notificationRef,
	}
}

func DecideFollowup(event PaymentEvent, now time.Time) FollowupDecision {
	due := !now.Before(event.NotBefore)
	state := "deferred"
	if due {
		state = "approved"
	}
	return FollowupDecision{
		Due:        due,
		Action:     event.Action,
		AuditEntry: fmt.Sprintf("payment=%s risk=%s action=%s state=%s notification=%s", event.PaymentID, event.RiskLevel, event.Action, state, event.NotificationRef),
	}
}

type FollowupWorker struct {
	queue *QueueClient
	now   func() time.Time
	audit func(FollowupDecision) error
}

func (w *FollowupWorker) RunOnce(ctx context.Context) ([]FollowupDecision, error) {
	messages, err := w.queue.Consume(ctx, 10, 60)
	if err != nil {
		return nil, err
	}
	decisions := make([]FollowupDecision, 0, len(messages))
	for _, message := range messages {
		var event PaymentEvent
		if err := json.Unmarshal(message.Payload, &event); err != nil {
			return decisions, fmt.Errorf("decode payment event %s: %w", message.MessageID, err)
		}
		decision := DecideFollowup(event, w.now())
		decisions = append(decisions, decision)
		if !decision.Due {
			continue
		}
		if err := w.audit(decision); err != nil {
			return decisions, err
		}
		if err := w.queue.Ack(ctx, message.MessageID); err != nil {
			return decisions, err
		}
	}
	return decisions, nil
}
