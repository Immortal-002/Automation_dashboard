package main

import (
	"fmt"
	"context"
	"net/http"
	"encoding/json"
    "os"
	"strings"
	"bufio"
	"bytes"
	"github.com/jackc/pgx/v5"
)

func ingestSampleData(conn *pgx.Conn) {
	docs := []string{
		"The Automation Dashboard is a distributed job execution system built in Go.",
		"Jobs are stored in PostgreSQL with task and log tables using foreign key constraints.",
		"Redis is used as an async job queue so the API server never blocks on execution.",
		"A separate Go worker process pulls jobs from Redis and runs shell commands using os/exec.",
		"The REST API exposes 5 endpoints: task CRUD, execute, and logs.",
		"JWT authentication is used to protect API endpoints.",
		"Prometheus metrics track request counts, error rates, and goroutine health.",
		"Grafana dashboards visualize the Prometheus metrics in real time.",
		"The GenAI layer adds LLM chat via Groq API with streaming SSE responses.",
		"Embeddings are generated locally using Ollama with the nomic-embed-text model.",
		"pgvector stores 768-dimensional embeddings and supports cosine similarity search.",
		"The RAG pipeline retrieves relevant chunks and grounds LLM answers in actual context.",
	}

	conn.Exec(context.Background(), "DELETE FROM documents")
	for _, doc := range docs {
		emb, err := getEmbedding(doc)
		if err != nil {
			fmt.Println("embedding error:", err)
			continue
		}
		insertDocument(conn, doc, emb)
	}
	fmt.Println("sample data ingested")
}

func buildRAGPrompt(chunks []string, question string) string {
	context := ""
	for i, chunk := range chunks {
		context += fmt.Sprintf("Chunk %d: %s\n", i+1, chunk)
	}

	return fmt.Sprintf(`You are a helpful assistant. Answer the question using ONLY the context provided below.
If the answer is not in the context, say "I don't have information about that."

Context:
%s
Question: %s`, context, question)
}

func handleRAG(w http.ResponseWriter, r *http.Request) {
    fmt.Println("RAG handler called") 
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
 fmt.Println("decoded request:", req.Message)


	// step 1: embed the question
	queryEmb, err := getEmbedding(req.Message)
	if err != nil {
		http.Error(w, "embedding failed", http.StatusInternalServerError)
		return
	}
 fmt.Println("embedding done, dims:", len(queryEmb))


	// step 2: retrieve relevant chunks
	conn, err := connectDB()
	if err != nil {
		http.Error(w, "db connection failed", http.StatusInternalServerError)
		return
	}
 fmt.Println("db connected")
	defer conn.Close(context.Background())

	chunks, err := searchDocuments(conn, queryEmb, 3)
	if err != nil {
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
 fmt.Println("chunks retrieved:", len(chunks))

	// step 3: build the prompt
	prompt := buildRAGPrompt(chunks, req.Message)

	// step 4: send to Groq and stream back
	groqReq := GroqRequest{
		Model: "llama-3.3-70b-versatile",
		Messages: []GroqMessage{
			{Role: "user", Content: prompt},
		},
		Stream: true,
	}
fmt.Println("api key set:", os.Getenv("GROQ_API_KEY") != "")

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
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		line = strings.TrimPrefix(line, "data: ")
		if line == "[DONE]" {
			break
		}

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

		fmt.Fprint(w, chunk.Choices[0].Delta.Content)
		flusher.Flush()
	}
}
