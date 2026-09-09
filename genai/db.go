package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
    pgvector "github.com/pgvector/pgvector-go"
)

func connectDB() (*pgx.Conn, error) {
	conn, err := pgx.Connect(context.Background(), "postgresql://postgres@localhost:5432/automation")
	if err != nil {
		return nil, fmt.Errorf("db connect failed: %w", err)
	}
	return conn, nil
}

func searchDocuments(conn *pgx.Conn, queryEmbedding []float64, limit int) ([]string, error) {
	vec := make([]float32, len(queryEmbedding))
	for i, v := range queryEmbedding {
		vec[i] = float32(v)
	}

	rows, err := conn.Query(context.Background(),
		"SELECT content FROM documents ORDER BY embedding <-> $1 LIMIT $2",
		pgvector.NewVector(vec), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		rows.Scan(&content)
		results = append(results, content)
	}
	return results, nil
}
func insertDocument(conn *pgx.Conn, content string, embedding []float64) error {
vec := make([]float32, len(embedding))
	for i, v := range embedding {
		vec[i] = float32(v)
	}
	_, err := conn.Exec(context.Background(),
		"INSERT INTO documents (content, embedding) VALUES ($1, $2)",
		content, pgvector.NewVector(vec),
	)
	return err
}
