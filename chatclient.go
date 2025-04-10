package lmstudio

import "github.com/mateuszmierzwinski/lmstudio/internal/model"

// ChatClient defines the interface for interacting with an LMStudio-compatible API.
type ChatClient interface {
	// ChatWithContext sends a message array with optional context ID and temperature,
	// returning the model's response messages.
	ChatWithContext(messages []model.LMAPIMessage, contextID string, temperature float64) ([]model.LMAPIMessage, error)

	// Chat is a simplified version of ChatWithContext that omits history/context.
	Chat(messages []model.LMAPIMessage, temperature float64) ([]model.LMAPIMessage, error)

	// LogicalCompressContext summarizes a list of messages using a built-in system prompt.
	LogicalCompressContext(messages []model.LMAPIMessage) (model.LMAPIMessage, error)

	// FlushMessageHistory removes all messages stored for the given context ID.
	FlushMessageHistory(historyID string)

	// ResetMessageHistory clears all stored contexts and associated messages.
	ResetMessageHistory()
}
