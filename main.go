package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	queue := NewQueueClient(key)
	worker := &FollowupWorker{
		queue: queue,
		now:   time.Now,
		audit: func(decision FollowupDecision) error {
			log.Printf("audit %s", decision.AuditEntry)
			return nil
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments/followups", scheduleHandler(queue))
	mux.HandleFunc("POST /workers/followups/run", workerHandler(worker))

	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("payment follow-up service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

type scheduleRequest struct {
	PaymentID       string `json:"payment_id"`
	RiskLevel       string `json:"risk_level"`
	NotificationRef string `json:"notification_ref"`
	DelayHours      int    `json:"delay_hours"`
}

func scheduleHandler(queue *QueueClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input scheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.PaymentID == "" || input.NotificationRef == "" || input.DelayHours < 1 {
			http.Error(w, "payment_id, notification_ref, and positive delay_hours are required", http.StatusBadRequest)
			return
		}
		event := NewPaymentEvent(input.PaymentID, input.RiskLevel, input.NotificationRef, time.Now(), time.Duration(input.DelayHours)*time.Hour)
		if err := queue.Publish(r.Context(), event, "payment-followup-"+input.PaymentID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, event)
	}
}

func workerHandler(worker *FollowupWorker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decisions, err := worker.RunOnce(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"decisions": decisions})
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	var apiErr *InfraiError
	if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
		writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Code})
		return
	}
	http.Error(w, "upstream request failed", http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("encode response: %v\n", err)
	}
}
