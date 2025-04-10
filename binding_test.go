package lmstudio

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mateuszmierzwinski/lmstudio/pkg/model"
)

// mockHTTPClient simulates an HTTP client response for testing.
type mockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

// helper to create JSON for model.LMStudioChatResponse
func mockResponseBody(content string) io.ReadCloser {
	response := `{
		"id": "mock-id",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "mock-model",
		"choices": [{
			"index": 0,
			"finish_reason": "stop",
			"message": {
				"role": "assistant",
				"content": "` + content + `"
			}
		}]
	}`
	return io.NopCloser(bytes.NewReader([]byte(response)))
}

func newTestBinding() *Binding {
	return &Binding{
		BaseURL:        "http://mockapi",
		AuthToken:      "",
		Model:          "mock-model",
		SessionID:      "test-session",
		Timeout:        5 * time.Second,
		HTTPClient:     &mockHTTPClient{},
		MessageHistory: make(map[string][]model.LMAPIMessage),
	}
}

func TestChatWithContext_AppendsToHistory(t *testing.T) {
	b := newTestBinding()

	// Inject mock response
	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       mockResponseBody("mocked response"),
			}, nil
		},
	}

	contextID := "test-ctx"
	input := []model.LMAPIMessage{{Role: "user", Content: "hello"}}
	resp, err := b.ChatWithContext(input, contextID, 0.7)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(resp) != 1 || resp[0].Content != "mocked response" {
		t.Errorf("unexpected response: %+v", resp)
	}

	history := b.MessageHistory[contextID]
	if len(history) != 1 || history[0].Content != "mocked response" {
		t.Errorf("expected history to be updated, got: %+v", history)
	}
}

func TestChat_ReturnsWithoutContext(t *testing.T) {
	b := newTestBinding()

	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       mockResponseBody("standalone response"),
			}, nil
		},
	}

	resp, err := b.Chat([]model.LMAPIMessage{{Role: "user", Content: "test"}}, 0.5)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	if len(resp) != 1 || resp[0].Content != "standalone response" {
		t.Errorf("unexpected chat response: %+v", resp)
	}
}

func TestLogicalCompressContext_WrapsProperly(t *testing.T) {
	b := newTestBinding()

	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       mockResponseBody("compressed summary"),
			}, nil
		},
	}

	msgs := []model.LMAPIMessage{
		{Role: "user", Content: "a lot of irrelevant info..."},
	}

	summary, err := b.LogicalCompressContext(msgs)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}
	if summary.Content != "compressed summary" {
		t.Errorf("unexpected summary: %+v", summary)
	}
}

func TestChatRequest_ErrorOnNon200(t *testing.T) {
	b := newTestBinding()

	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(bytes.NewBufferString("pkg error")),
			}, nil
		},
	}

	_, err := b.ChatWithContext([]model.LMAPIMessage{{Role: "user", Content: "fail"}}, "", 0.5)
	if err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

// TestFlushMessageHistory ensures a specific context is removed from history.
func TestFlushMessageHistory(t *testing.T) {
	b := newTestBinding()
	ctx := "flush-id"
	b.MessageHistory[ctx] = []model.LMAPIMessage{{Role: "user", Content: "hello"}}

	b.FlushMessageHistory(ctx)

	if _, exists := b.MessageHistory[ctx]; exists {
		t.Errorf("expected context %s to be flushed", ctx)
	}
}

// TestResetMessageHistory clears all entries.
func TestResetMessageHistory(t *testing.T) {
	b := newTestBinding()
	b.MessageHistory["ctx1"] = []model.LMAPIMessage{{Role: "user", Content: "test"}}
	b.MessageHistory["ctx2"] = []model.LMAPIMessage{{Role: "assistant", Content: "response"}}

	b.ResetMessageHistory()

	if len(b.MessageHistory) != 0 {
		t.Errorf("expected message history to be empty, got %d entries", len(b.MessageHistory))
	}
}

// TestPersistAndLoadHistory validates writing and reading persisted history.
func TestPersistAndLoadHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "history.json")

	b := newTestBinding()
	b.PersistHistoryFile = historyFile
	b.MessageHistory["ctx"] = []model.LMAPIMessage{{Role: "user", Content: "hello"}}

	if err := b.persistHistory(); err != nil {
		t.Fatalf("failed to persist history: %v", err)
	}

	// create a new binding and load from file
	b2 := newTestBinding()
	b2.PersistHistoryFile = historyFile
	if err := b2.loadPersistedHistory(); err != nil {
		t.Fatalf("failed to load persisted history: %v", err)
	}

	history := b2.MessageHistory["ctx"]
	if len(history) != 1 || history[0].Content != "hello" {
		t.Errorf("unexpected loaded history: %+v", history)
	}
}

// TestPersistHistory_NoFilePath ensures persistHistory does nothing when file is not set.
func TestPersistHistory_NoFilePath(t *testing.T) {
	b := newTestBinding()
	b.MessageHistory["ctx"] = []model.LMAPIMessage{{Role: "user", Content: "hello"}}
	b.PersistHistoryFile = ""

	err := b.persistHistory()
	if err != nil {
		t.Errorf("expected nil error with empty path, got: %v", err)
	}
}

// TestLoadHistory_FileNotExist returns nil if file is missing
func TestLoadHistory_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	missingFile := filepath.Join(tmpDir, "missing.json")

	b := newTestBinding()
	b.PersistHistoryFile = missingFile

	err := b.loadPersistedHistory()
	if err != nil {
		t.Errorf("expected nil error on missing file, got: %v", err)
	}
}

// TestChatRequest_EmptyChoices triggers empty response error
func TestChatRequest_EmptyChoices(t *testing.T) {
	b := newTestBinding()
	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			emptyResp := `{
				"id": "x", "object": "chat.completion", "created": 0, "model": "mock",
				"choices": []
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(emptyResp)),
			}, nil
		},
	}

	_, err := b.Chat([]model.LMAPIMessage{{Role: "user", Content: "test"}}, 0.7)
	if err == nil || err.Error() != "empty response from model" {
		t.Errorf("expected empty response error, got: %v", err)
	}
}

// TestChatRequest_BadJSON triggers decode error
func TestChatRequest_BadJSON(t *testing.T) {
	b := newTestBinding()
	b.HTTPClient = &mockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("not-json")),
			}, nil
		},
	}

	_, err := b.Chat([]model.LMAPIMessage{{Role: "user", Content: "fail"}}, 0.5)
	if err == nil || err.Error() == "" {
		t.Error("expected JSON decode error, got nil")
	}
}

// TestChatWithContext_PersistsHistory ensures that after a chat,
// history is written to disk automatically.
func TestChatWithContext_PersistsHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "persisted.json")

	b := &Binding{
		BaseURL:            "http://mockapi",
		AuthToken:          "",
		Model:              "mock-model",
		SessionID:          "test-session",
		Timeout:            5 * time.Second,
		PersistHistoryFile: historyFile,
		HTTPClient: &mockHTTPClient{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       mockResponseBody("persisted message"),
				}, nil
			},
		},
		MessageHistory: make(map[string][]model.LMAPIMessage),
	}

	_, err := b.ChatWithContext([]model.LMAPIMessage{{Role: "user", Content: "save me"}}, "my-session", 0.6)
	if err != nil {
		t.Fatalf("ChatWithContext failed: %v", err)
	}

	// Confirm file exists and has data
	data, err := os.ReadFile(historyFile)
	if err != nil {
		t.Fatalf("Failed to read persisted file: %v", err)
	}

	var loaded map[string][]model.LMAPIMessage
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	msgs := loaded["my-session"]
	if len(msgs) == 0 || msgs[0].Content != "persisted message" {
		t.Errorf("Expected persisted message, got: %+v", msgs)
	}
}

// TestNewLMStudioBinding_LoadsExistingHistory verifies that loading happens at construction.
func TestNewLMStudioBinding_LoadsExistingHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "preloaded.json")

	preloaded := map[string][]model.LMAPIMessage{
		"restore-session": {
			{Role: "assistant", Content: "previous message"},
		},
	}
	raw, _ := json.Marshal(preloaded)
	if err := os.WriteFile(historyFile, raw, 0644); err != nil {
		t.Fatalf("Failed to prewrite history: %v", err)
	}

	b, err := NewLMStudioBinding(
		"http://mockapi",
		"",
		"mock-model",
		historyFile,
		5*time.Second,
		"restore-session",
	)
	if err != nil {
		t.Fatalf("Failed to initialize binding: %v", err)
	}

	history := b.MessageHistory["restore-session"]
	if len(history) != 1 || history[0].Content != "previous message" {
		t.Errorf("Failed to load history from file: %+v", history)
	}
}
