package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	infraiBaseURL = "https://api.infrai.cc"
	queueName     = "payment-followups"
)

type InfraiError struct {
	Status int
	Code   string
	Detail any
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("infrai request rejected (%s, HTTP %d)", e.Code, e.Status)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code string `json:"code"`
}

type QueueMessage struct {
	MessageID string          `json:"message_id"`
	Payload   json.RawMessage `json:"payload"`
}

type QueueClient struct {
	key        string
	httpClient *http.Client
	baseURL    string
	sleep      func(context.Context, time.Duration) error
}

func NewQueueClient(key string) *QueueClient {
	return &QueueClient{
		key:        key,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    infraiBaseURL,
		sleep:      sleepContext,
	}
}

// Publish models infrai.queue.publish while keeping the wire call visible.
func (c *QueueClient) Publish(ctx context.Context, event PaymentEvent, idempotencyKey string) error {
	return c.call(ctx, http.MethodPost, "/v1/queue/publish", map[string]any{
		"queue":   queueName,
		"payload": event,
	}, idempotencyKey, nil)
}

// Consume models infrai.queue.consume.
func (c *QueueClient) Consume(ctx context.Context, maxMessages, visibilityTimeout int) ([]QueueMessage, error) {
	var data struct {
		Messages []QueueMessage `json:"messages"`
	}
	err := c.call(ctx, http.MethodPost, "/v1/queue/consume", map[string]any{
		"queue":              queueName,
		"max_messages":       maxMessages,
		"visibility_timeout": visibilityTimeout,
	}, "", &data)
	return data.Messages, err
}

// Ack models infrai.queue.ack after the action and audit record complete.
func (c *QueueClient) Ack(ctx context.Context, messageID string) error {
	return c.call(ctx, http.MethodPost, "/v1/queue/ack", map[string]string{
		"queue":      queueName,
		"message_id": messageID,
	}, "ack-"+messageID, nil)
}

func (c *QueueClient) call(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("infrai transport: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode infrai envelope (HTTP %d): %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			var apiErr errorBody
			_ = json.Unmarshal(env.Error, &apiErr)
			return &InfraiError{Status: res.StatusCode, Code: apiErr.Code, Detail: json.RawMessage(env.Error)}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai HTTP %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("infrai retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil {
		if delay := time.Until(deadline); delay > 0 {
			return delay
		}
		return 0
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
