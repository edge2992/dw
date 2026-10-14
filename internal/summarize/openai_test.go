package summarize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestOpenAI(url, key string) *openAI {
	return &openAI{baseURL: strings.TrimRight(url, "/"), apiKey: key, model: "cheap", client: &http.Client{}, backoff: []time.Duration{time.Millisecond, time.Millisecond}}
}

func reply(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
	})
}

func TestOpenAI_RequestShape(t *testing.T) {
	for _, withKey := range []bool{true, false} {
		var gotPath, gotAuth string
		var body chatRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&body)
			reply(w, "# T\n")
		}))
		key := ""
		if withKey {
			key = "secret"
		}
		// A trailing slash on the base URL must not double up.
		o := newTestOpenAI(srv.URL+"/v1/", key)
		out, err := o.Summarize(context.Background(), Request{Date: "2026-10-15", State: "# T\n", Transcript: "USER: hi\n"})
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if out != "# T\n" {
			t.Errorf("out = %q", out)
		}
		if gotPath != "/v1/chat/completions" {
			t.Errorf("path = %q", gotPath)
		}
		if withKey && gotAuth != "Bearer secret" || !withKey && gotAuth != "" {
			t.Errorf("Authorization = %q with key=%v", gotAuth, withKey)
		}
		if body.Model != "cheap" || len(body.Messages) != 2 || body.Messages[0].Role != "system" ||
			!strings.Contains(body.Messages[1].Content, "USER: hi") || !strings.Contains(body.Messages[1].Content, "2026-10-15") {
			t.Errorf("body = %+v", body)
		}
	}
}

func TestOpenAI_RetriesOnlyTransientErrors(t *testing.T) {
	cases := []struct {
		status    int
		wantCalls int32
		wantOK    bool
	}{
		{http.StatusTooManyRequests, 2, true}, // fails once, then succeeds
		{http.StatusBadGateway, 2, true},
		{http.StatusBadRequest, 1, false},
		{http.StatusUnauthorized, 1, false},
	}
	for _, c := range cases {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				http.Error(w, strings.Repeat("x", 500), c.status)
				return
			}
			reply(w, "ok")
		}))
		_, err := newTestOpenAI(srv.URL, "").Summarize(context.Background(), Request{})
		srv.Close()
		if (err == nil) != c.wantOK || calls.Load() != c.wantCalls {
			t.Errorf("status %d: err=%v calls=%d, want ok=%v calls=%d", c.status, err, calls.Load(), c.wantOK, c.wantCalls)
		}
		if err != nil && len(err.Error()) > 250 {
			t.Errorf("status %d: error body not truncated: %d bytes", c.status, len(err.Error()))
		}
	}
}

func TestOpenAI_GivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := newTestOpenAI(srv.URL, "").Summarize(context.Background(), Request{}); err == nil {
		t.Fatal("want an error")
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 1 + 2 retries", calls.Load())
	}
}

func TestOpenAI_ContextTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release) // runs before srv.Close so a lingering handler cannot block it
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := newTestOpenAI(srv.URL, "").Summarize(ctx, Request{}); err == nil {
		t.Fatal("want a timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Error("timeout not honoured")
	}
}

func TestOpenAI_OversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(w, strings.Repeat("x", 2<<20))
	}))
	defer srv.Close()
	if _, err := newTestOpenAI(srv.URL, "").Summarize(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("err = %v, want the size cap", err)
	}
}
