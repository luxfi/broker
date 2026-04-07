// Package taskqueue provides an HTTP client for the Hanzo Tasks durable queue.
//
// When TASKS_URL is set, tasks are enqueued via HTTP POST. When unset, the
// caller falls back to direct execution (dev mode).
package taskqueue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Client is a thin HTTP client for the Hanzo Tasks queue.
type Client struct {
	tasksURL   string
	httpClient *http.Client
}

// Task is the envelope posted to the Tasks HTTP API.
type Task struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// New reads TASKS_URL from the environment and returns a Client.
// Returns nil if TASKS_URL is not set (dev mode — callers should fall back
// to direct execution).
func New() *Client {
	url := os.Getenv("TASKS_URL")
	if url == "" {
		return nil
	}
	return &Client{
		tasksURL:   url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewWithURL creates a Client targeting the given Tasks URL.
// Useful for testing without environment variables.
func NewWithURL(url string) *Client {
	if url == "" {
		return nil
	}
	return &Client{
		tasksURL:   url,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enqueue posts a task to the Hanzo Tasks HTTP API.
func (c *Client) Enqueue(taskType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("taskqueue: marshal payload: %w", err)
	}

	task := Task{
		Type:    taskType,
		Payload: raw,
	}
	body, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue: marshal task: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.tasksURL+"/v1/tasks", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("taskqueue: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("taskqueue: http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("taskqueue: unexpected status %d", resp.StatusCode)
	}

	return nil
}
