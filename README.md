# lmstudio

A lightweight, dependency-free Go client for communicating with LMStudio/OpenAI-compatible Chat APIs.  
It provides message history tracking, context-aware conversations, and logical compression of context to help reduce token usage efficiently.

---

## ✨ Features

- 📡 Send chat requests to LMStudio-compatible APIs
- 💬 Support for context-based conversations using in-memory history
- 🧠 Built-in message summarization via logical compression
- 🧪 Mockable interface for testing (`ChatClient`)
- 💾 Optional message history persistence
- ⚙️ Uses **only the Go standard library** — no external dependencies

---

## 📦 Installation

```bash
go get github.com/mateuszmierzwinski/lmstudio
```

---

## 🔧 Quick Start

### Create a New Binding

```go
package main

import (
	"fmt"
	"github.com/mateuszmierzwinski/lmstudio"
	"github.com/mateuszmierzwinski/lmstudio/internal/model"
)

func main() {
	client := lmstudio.NewLMStudioQuickBind(
		"http://localhost:1234", // LMStudio or compatible API base URL
		"gpt-3.5-turbo",         // Model name
		"",                      // Optional session ID
	)

	msg := []model.LMAPIMessage{
		{Role: "user", Content: "Tell me a story about a dragon."},
	}

	resp, err := client.Chat(msg, 0.7)
	if err != nil {
		panic(err)
	}

	for _, r := range resp {
		fmt.Println("Assistant:", r.Content)
	}
}
```

---

## 🧠 Using Context History

```go
contextID := "session-1"

response, err := client.ChatWithContext([]model.LMAPIMessage{
	{Role: "user", Content: "Who was Alan Turing?"},
}, contextID, 0.6)

if err != nil {
	log.Fatal(err)
}

fmt.Println(response[0].Content)
```

---

## 🔍 Compressing History (Summarization)

When conversation grows long, compress it using the built-in logic:

```go
history := client.MessageHistory["session-1"]

summary, err := client.LogicalCompressContext(history)
if err != nil {
	log.Fatal(err)
}

fmt.Println("Compressed Summary:", summary.Content)
```

---

## ✅ Interface for Testing

Use the `ChatClient` interface to easily mock the binding:

```go
type ChatClient interface {
	ChatWithContext([]model.LMAPIMessage, string, float64) ([]model.LMAPIMessage, error)
	Chat([]model.LMAPIMessage, float64) ([]model.LMAPIMessage, error)
	LogicalCompressContext([]model.LMAPIMessage) (model.LMAPIMessage, error)
	FlushMessageHistory(string)
	ResetMessageHistory()
}
```

---

## 🧪 Philosophy

This module is intentionally **minimal and dependency-free**.  
It relies only on Go’s standard library (`net/http`, `encoding/json`, `os`, etc.), making it ideal for secure, auditable, and lightweight environments.

---

## 📁 Project Structure

```
.
├── internal/
│   └── model/            # API request/response types
├── lmstudio/             # Core logic (binding, interface)
├── go.mod
└── README.md
```

---

## 🛠 Advanced Configuration

Create a fully customized binding:

```go
binding := lmstudio.NewLMStudioBinding(
	"http://localhost:1234", // Base URL
	"your-auth-token",       // Auth token (optional)
	"gpt-3.5-turbo",         // Model name
	"./history.json",        // Optional history persistence file
	10*time.Second,          // Timeout
	""                       // Session ID (auto-generated if empty)
)
```

---

## 📜 License

MIT © [Mateusz Mierzwinski](LICENSE.md)