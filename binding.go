package lmstudio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mateuszmierzwinski/lmstudio/internal/model"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	emptyContextID        = ""
	chatCompletionsURI    = "/v1/chat/completions"
	logicalCompressPrompt = "Compress logically (summarize) context of this discussion so it will contain only important details dropping noise for further discussion. Return only single message."

	roleSystem                   = "system"
	summarizeCompressTemperature = 0.3

	defaultAuthToken          = ""
	defaultHistoryPersistFile = ""
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Binding provides access to LMStudio's API, managing session state, request logic,
// and in-memory message history with optional persistence.
type Binding struct {
	BaseURL    string
	AuthToken  string
	Model      string
	SessionID  string
	HTTPClient HTTPClient
	Timeout    time.Duration

	// MessageHistory stores context history per conversation ID.
	// While LMStudio accepts prior messages as input, maintaining history locally
	// can reduce token usage. Consider using LogicalCompressContext to condense
	// history before resending it to the model.
	MessageHistory map[string][]model.LMAPIMessage

	PersistHistoryFile string
	messageHistoryMtx  sync.RWMutex
}

// ChatWithContext sends a chat completion request with optional context history.
// If contextID is provided, it prepends prior messages and appends new responses to history.
func (l *Binding) ChatWithContext(messages []model.LMAPIMessage, contextID string, temperature float64) ([]model.LMAPIMessage, error) {
	if contextID != "" && l.MessageHistory != nil {
		l.messageHistoryMtx.RLock()
		messages = append(l.MessageHistory[contextID], messages...)
		l.messageHistoryMtx.RUnlock()
	}

	response, err := l.chatRequest(chatCompletionsURI, messages, temperature)
	if err != nil {
		return nil, err
	}

	if contextID != "" && l.MessageHistory != nil {
		l.messageHistoryMtx.Lock()
		l.MessageHistory[contextID] = append(l.MessageHistory[contextID], response...)
		l.messageHistoryMtx.Unlock()

		if err = l.persistHistory(); err != nil {
			return response, fmt.Errorf("error persisting history: %x", err)
		}
	}

	return response, nil
}

// chatRequest sends a low-level request to the LMStudio API and parses the result.
// It wraps the model, messages, and temperature into a valid payload.
func (l *Binding) chatRequest(uri string, messages []model.LMAPIMessage, temperature float64) ([]model.LMAPIMessage, error) {
	request := model.LMStudioChatRequest{
		Model:       l.Model,
		Messages:    messages,
		Temperature: temperature,
	}

	endpoint := fmt.Sprintf("%s%s", l.BaseURL, uri)

	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if l.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.AuthToken)
	}

	resp, err := l.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, errMsg)
	}

	var chatResp model.LMStudioChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("empty response from model")
	}

	messagesOut := make([]model.LMAPIMessage, len(chatResp.Choices))
	for i, choice := range chatResp.Choices {
		messagesOut[i] = choice.Message
	}

	return messagesOut, nil
}

// Chat sends messages to the API without context or history.
// It's a simplified wrapper over ChatWithContext.
func (l *Binding) Chat(messages []model.LMAPIMessage, temperature float64) ([]model.LMAPIMessage, error) {
	return l.ChatWithContext(messages, emptyContextID, temperature)
}

// LogicalCompressContext summarizes a discussion using the LM model.
// It prepends an instruction message and returns the final response.
func (l *Binding) LogicalCompressContext(message []model.LMAPIMessage) (model.LMAPIMessage, error) {
	summaryContext := []model.LMAPIMessage{
		{
			Role:    roleSystem,
			Content: logicalCompressPrompt,
		},
	}

	summaryContext = append(summaryContext, message...)
	result, err := l.Chat(summaryContext, summarizeCompressTemperature)
	if err != nil {
		return model.LMAPIMessage{}, err
	}
	return result[len(result)-1], nil
}

// FlushMessageHistory deletes the stored message history for a specific context ID.
func (l *Binding) FlushMessageHistory(historyID string) {
	l.messageHistoryMtx.Lock()
	defer l.messageHistoryMtx.Unlock()
	delete(l.MessageHistory, historyID)
}

// ResetMessageHistory clears all stored message histories.
func (l *Binding) ResetMessageHistory() {
	l.messageHistoryMtx.Lock()
	defer l.messageHistoryMtx.Unlock()
	l.MessageHistory = make(map[string][]model.LMAPIMessage)
}

// persistHistory writes the current in-memory message history to a file, if configured.
func (l *Binding) persistHistory() error {
	if l.PersistHistoryFile != "" && l.MessageHistory != nil {
		l.messageHistoryMtx.RLock()
		defer l.messageHistoryMtx.RUnlock()
		f, err := os.OpenFile(l.PersistHistoryFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		return json.NewEncoder(f).Encode(l.MessageHistory)
	}
	return nil
}

// loadPersistedHistory reads message history from a file into memory, if configured.
func (l *Binding) loadPersistedHistory() error {
	if l.PersistHistoryFile != "" {
		if l.MessageHistory == nil {
			l.messageHistoryMtx.Lock()
			l.MessageHistory = make(map[string][]model.LMAPIMessage)
			l.messageHistoryMtx.Unlock()
		}
		if _, err := os.Stat(l.PersistHistoryFile); err != nil {
			return nil // File doesn't exist yet; not an error
		}

		f, err := os.OpenFile(l.PersistHistoryFile, os.O_RDONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		l.messageHistoryMtx.Lock()
		defer l.messageHistoryMtx.Unlock()

		return json.NewDecoder(f).Decode(&l.MessageHistory)
	}
	return nil
}

// NewLMStudioQuickBind returns a Binding instance with default token, file, and timeout values.
func NewLMStudioQuickBind(baseURL, aiModel, sessionID string) (*Binding, error) {
	return NewLMStudioBinding(baseURL, defaultAuthToken, aiModel, defaultHistoryPersistFile, 5*time.Second, sessionID)
}

// NewLMStudioBinding initializes a Binding instance with custom configuration.
// If sessionID is empty, it generates a timestamp-based session name.
func NewLMStudioBinding(baseURL, authToken, aiModel, persistHistoryFile string, timeout time.Duration, sessionID string) (*Binding, error) {
	if sessionID == "" {
		sessionID = fmt.Sprintf("lmstudio-binding-session-%d", time.Now().UnixNano())
	}
	b := &Binding{
		BaseURL:            baseURL,
		AuthToken:          authToken,
		Model:              aiModel,
		SessionID:          sessionID,
		PersistHistoryFile: persistHistoryFile,
		Timeout:            timeout,
		HTTPClient:         &http.Client{Timeout: timeout},
		MessageHistory:     make(map[string][]model.LMAPIMessage),
	}

	if err := b.loadPersistedHistory(); err != nil {
		return nil, err
	}

	return b, nil
}
