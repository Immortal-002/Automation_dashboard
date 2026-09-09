
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"math"
)
type EmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}
type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

func getEmbedding(text string) ([]float64, error) {
	reqBody := EmbeddingRequest{
		Model:  "nomic-embed-text",
		Prompt: text,
	}
    body, _ := json.Marshal(reqBody)
	resp, err := http.Post(
		"http://localhost:11434/api/embeddings",
		"application/json",
		bytes.NewBuffer(body),
	)
    if err != nil {
		return nil, fmt.Errorf("ollama call failed: %w", err)
	}
	defer resp.Body.Close()

	var embResp EmbeddingResponse
	json.NewDecoder(resp.Body).Decode(&embResp)

	return embResp.Embedding, nil
}
func cosineSimilarity(a, b []float64) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
