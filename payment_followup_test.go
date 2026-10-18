package main

import (
	"strings"
	"testing"
	"time"
)

func TestDecideFollowup(t *testing.T) {
	base := time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		risk       string
		now        time.Time
		wantDue    bool
		wantAction string
		wantState  string
	}{
		{name: "normal payment remains deferred before four hours", risk: "normal", now: base.Add(3*time.Hour + 59*time.Minute), wantDue: false, wantAction: "send-payment-reminder", wantState: "state=deferred"},
		{name: "normal payment reminder is due at four hours", risk: "normal", now: base.Add(4 * time.Hour), wantDue: true, wantAction: "send-payment-reminder", wantState: "state=approved"},
		{name: "high risk payment routes to review", risk: "high", now: base.Add(5 * time.Hour), wantDue: true, wantAction: "request-manual-review", wantState: "state=approved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := NewPaymentEvent("pay_1042", tt.risk, "notice_82", base, 4*time.Hour)
			got := DecideFollowup(event, tt.now)
			if got.Due != tt.wantDue || got.Action != tt.wantAction || !strings.Contains(got.AuditEntry, tt.wantState) {
				t.Fatalf("DecideFollowup() = %+v", got)
			}
			if !strings.Contains(got.AuditEntry, "payment=pay_1042") || !strings.Contains(got.AuditEntry, "notification=notice_82") {
				t.Fatalf("audit entry lacks trace references: %q", got.AuditEntry)
			}
		})
	}
}
