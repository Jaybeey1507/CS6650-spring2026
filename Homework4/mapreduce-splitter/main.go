package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type SplitResponse struct {
	InputS3   string   `json:"input_s3"`
	ChunkS3   []string `json:"chunk_s3_urls"`
	ChunkKeys []string `json:"chunk_keys"`
	RunID     string   `json:"run_id"`
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9']+`)

func tokenize(s string) []string {
	matches := wordRe.FindAllString(s, -1)
	out := make([]string, 0, len(matches))
	for _, w := range matches {
		w = strings.ToLower(w)
		out = append(out, w)
	}
	return out
}

func splitTokens(tokens []string, parts int) [][]string {
	if parts <= 0 {
		parts = 1
	}
	chunks := make([][]string, parts)
	n := len(tokens)
	base := n / parts
	rem := n % parts

	idx := 0
	for i := 0; i < parts; i++ {
		size := base
		if i < rem {
			size++
		}
		if idx+size > n {
			size = n - idx
		}
		chunks[i] = tokens[idx : idx+size]
		idx += size
	}
	return chunks
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/split", func(w http.ResponseWriter, r *http.Request) {
		// Expected query: /split?bucket=...&key=input.txt
		bucket := r.URL.Query().Get("bucket")
		key := r.URL.Query().Get("key")
		if bucket == "" || key == "" {
			http.Error(w, "missing bucket or key. usage: /split?bucket=YOUR_BUCKET&key=input.txt", http.StatusBadRequest)
			return
		}

		ctx := context.Background()

		awsCfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			http.Error(w, "aws config error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s3c := s3.NewFromConfig(awsCfg)

		// Download file from S3
		obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})
		if err != nil {
			http.Error(w, "getobject error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer obj.Body.Close()

		// Read content (portable across Go versions; avoids strings.Builder.ReadFrom)
		data, err := io.ReadAll(obj.Body)
		if err != nil {
			http.Error(w, "read body error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tokens := tokenize(string(data))
		if len(tokens) == 0 {
			http.Error(w, "no tokens found in file", http.StatusBadRequest)
			return
		}

		runID := fmt.Sprintf("run-%d", time.Now().Unix())
		parts := 3
		chunks := splitTokens(tokens, parts)

		chunkURLs := make([]string, 0, parts)
		chunkKeys := make([]string, 0, parts)

		for i := 0; i < parts; i++ {
			chunkKey := fmt.Sprintf("chunks/%s/chunk-%d.txt", runID, i+1)
			body := strings.NewReader(strings.Join(chunks[i], " "))

			_, err := s3c.PutObject(ctx, &s3.PutObjectInput{
				Bucket:      &bucket,
				Key:         &chunkKey,
				Body:        body,
				ContentType: strPtr("text/plain"),
			})
			if err != nil {
				http.Error(w, "putobject error: "+err.Error(), http.StatusInternalServerError)
				return
			}

			chunkKeys = append(chunkKeys, chunkKey)
			chunkURLs = append(chunkURLs, fmt.Sprintf("s3://%s/%s", bucket, chunkKey))
		}

		resp := SplitResponse{
			InputS3:   fmt.Sprintf("s3://%s/%s", bucket, key),
			ChunkS3:   chunkURLs,
			ChunkKeys: chunkKeys,
			RunID:     runID,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	log.Printf("splitter listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func strPtr(s string) *string { return &s }
