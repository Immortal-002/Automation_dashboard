package main

import (
	"bytes"
    "encoding/json"
    "fmt"
    "net/http"
	"os"
	"strings"
	"bufio"
//	"io"
)

type ChatRequest struct {
    Message string `json:"message"`
}
type GroqRequest struct {
	Model    string        `json:"model"`
	Messages []GroqMessage `json:"messages"`
	Stream bool            `json:"stream"`
}
type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}


var conversationHistory = []GroqMessage{
	{Role: "system", Content: "You are a concise technical assistant."},
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    var req ChatRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }
    conversationHistory = append(conversationHistory, GroqMessage{
    Role:    "user",
    Content: req.Message,
    })

	groqReq := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: conversationHistory,
		Stream: true,
	}

	body, _ := json.Marshal(groqReq)
	httpReq, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("GROQ_API_KEY"))

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "groq call failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()


	scanner := bufio.NewScanner(resp.Body)
var fullResponse string

for scanner.Scan() {
    line := scanner.Text()
  //  fmt.Println("LINE:", line)

    // skip empty lines
    if line == "" {
        continue
    }

    // strip the "data: " prefix
    if !strings.HasPrefix(line, "data: ") {
        continue
    }
    line = strings.TrimPrefix(line, "data: ")

    // end of stream
    if line == "[DONE]" {
        break
    }

    // parse the chunk
    var chunk struct {
        Choices []struct {
            Delta struct {
                Content string `json:"content"`
            } `json:"delta"`
        } `json:"choices"`
    }
    if err := json.Unmarshal([]byte(line), &chunk); err != nil {
        continue
    }

    if len(chunk.Choices) == 0 {
        continue
    }

    token := chunk.Choices[0].Delta.Content
    fullResponse += token

    // write and flush immediately
    fmt.Fprint(w, token)
    flusher.Flush()
}	
conversationHistory = append(conversationHistory, GroqMessage{
    Role:    "assistant",
    Content: fullResponse,
})

   }



func main() {
    emb, err := getEmbedding("table")
    if err != nil {
        fmt.Println("error:", err)
    } else {
        fmt.Printf("dimensions: %d\n", len(emb))
        fmt.Printf("first 3 values: %.4f, %.4f, %.4f\n", emb[0], emb[1], emb[2])
    }
    http.HandleFunc("/chat", handleChat)
    fmt.Println("server running on :8080")
    http.ListenAndServe(":8080", nil)
}
