package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// openAI talks to any OpenAI-compatible POST {base_url}/chat/completions
// endpoint (OpenRouter, Ollama, llama.cpp, vLLM, vendor compat layers).
type openAI struct {
	baseURL, apiKey, model string
	client                 *http.Client
	backoff                []time.Duration // waits before each retry; len = retry count
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest deliberately sends no temperature or max_tokens: several
// reasoning-model endpoints reject one or the other, and output size is
// bounded by validation anyway.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (o *openAI) Name() string { return "openai/" + o.model }

// Summarize retries 429, 5xx and network errors with backoff; any other
// failure (bad key, bad model) is returned at once.
func (o *openAI) Summarize(ctx context.Context, req Request) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage(req)},
		},
	})
	if err != nil {
		return "", err
	}
	for attempt := 0; ; attempt++ {
		out, retry, err := o.post(ctx, body)
		if err == nil || !retry || attempt >= len(o.backoff) {
			return out, err
		}
		select {
		case <-ctx.Done():
			return "", err
		case <-time.After(o.backoff[attempt]):
		}
	}
}

func (o *openAI) post(ctx context.Context, body []byte) (out string, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return "", ctx.Err() == nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", true, err
	}
	if resp.StatusCode/100 != 2 {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", retry, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	if len(b) > maxResponseBytes {
		return "", false, errors.New("response larger than 1 MiB")
	}
	var r chatResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return "", false, fmt.Errorf("decode response: %w", err)
	}
	if len(r.Choices) == 0 {
		return "", false, errors.New("response has no choices")
	}
	return r.Choices[0].Message.Content, false, nil
}
