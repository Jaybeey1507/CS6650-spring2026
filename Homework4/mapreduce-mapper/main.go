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
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type MapResponse struct {
	InputS3   string `json:"input_s3"`
	OutputS3  string `json:"output_s3"`
	OutputKey string `json:"output_key"`
	RunID     string `json:"run_id"`
	Top10     []Pair `json:"top10"`
}

type Pair struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9']+`)

func tokenize(s string) []string {
	matches := wordRe.FindAllString(s, -1)
	out := make([]string, 0, len(matches))
	for _, w := range matches {
		out = append(out, strings.ToLower(w))
	}
	return out
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		// /map?bucket=...&key=chunks/.../chunk-1.txt&run=run-...&id=1
		bucket := r.URL.Query().Get("bucket")
		key := r.URL.Query().Get("key")
		runID := r.URL.Query().Get("run")
		mapperID := r.URL.Query().Get("id")

		if bucket == "" || key == "" {
			http.Error(w, "missing bucket or key. usage: /map?bucket=YOUR_BUCKET&key=CHUNK_KEY&run=RUN_ID&id=1", http.StatusBadRequest)
			return
		}
		if runID == "" {
			runID = fmt.Sprintf("run-%d", time.Now().Unix())
		}
		if mapperID == "" {
			mapperID = "1"
		}

		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			http.Error(w, "aws config error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s3c := s3.NewFromConfig(awsCfg)

		obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})
		if err != nil {
			http.Error(w, "getobject error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer obj.Body.Close()

		data, err := io.ReadAll(obj.Body)
		if err != nil {
			http.Error(w, "read body error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		tokens := tokenize(string(data))
		if len(tokens) == 0 {
			http.Error(w, "no tokens found in chunk", http.StatusBadRequest)
			return
		}

		counts := map[string]int{}
		for _, t := range tokens {
			counts[t]++
		}

		// Write mapper output JSON to S3
		outKey := fmt.Sprintf("maps/%s/mapper-%s.json", runID, mapperID)
		outBytes, _ := json.Marshal(counts)

		_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      &bucket,
			Key:         &outKey,
			Body:        strings.NewReader(string(outBytes)),
			ContentType: strPtr("application/json"),
		})
		if err != nil {
			http.Error(w, "putobject error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Build top10 for easier screenshots
		pairs := make([]Pair, 0, len(counts))
		for k, v := range counts {
			pairs = append(pairs, Pair{Word: k, Count: v})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].Count == pairs[j].Count {
				return pairs[i].Word < pairs[j].Word
			}
			return pairs[i].Count > pairs[j].Count
		})
		if len(pairs) > 10 {
			pairs = pairs[:10]
		}

		resp := MapResponse{
			InputS3:   fmt.Sprintf("s3://%s/%s", bucket, key),
			OutputS3:  fmt.Sprintf("s3://%s/%s", bucket, outKey),
			OutputKey: outKey,
			RunID:     runID,
			Top10:     pairs,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	log.Printf("mapper listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func strPtr(s string) *string { return &s }
