package model

// LMAPIMessage is LMStudio API message format
type LMAPIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LMStudioChatRequest to /v1/chat/completions
type LMStudioChatRequest struct {
	Model       string         `json:"model"`
	Messages    []LMAPIMessage `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
}

// LMStudioChatChoice holds a response
type LMStudioChatChoice struct {
	Index        int          `json:"index"`
	FinishReason string       `json:"finish_reason"`
	Message      LMAPIMessage `json:"message"`
}

// LMStudioChatResponse from API
type LMStudioChatResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []LMStudioChatChoice `json:"choices"`
}
