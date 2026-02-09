package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type MapperKV struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type ReduceResponse struct {
	InputMapperS3 []string   `json:"input_mapper_s3"`
	OutputS3      string     `json:"output_s3"`
	OutputKey     string     `json:"output_key"`
	RunID         string     `json:"run_id"`
	Top10         []MapperKV `json:"top10"`
	TotalWords    int        `json:"total_words"`
	UniqueWords   int        `json:"unique_words"`
}

// decodeCountsFlexible accepts BOTH:
// 1) {"counts": {"a":1,"b":2}}
// 2) {"a":1,"b":2}  (your current mapper output)
func decodeCountsFlexible(data []byte) (map[string]int, error) {
	// Try wrapped format first
	var wrapped struct {
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Counts != nil {
		return wrapped.Counts, nil
	}

	// Try flat map format
	var flat map[string]int
	if err := json.Unmarshal(data, &flat); err == nil && flat != nil {
		return flat, nil
	}

	return nil, fmt.Errorf("unsupported mapper json format")
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/reduce", func(w http.ResponseWriter, r *http.Request) {
		// /reduce?bucket=YOUR_BUCKET&keys=key1,key2,key3
		bucket := r.URL.Query().Get("bucket")
		keysParam := r.URL.Query().Get("keys")

		if bucket == "" || keysParam == "" {
			http.Error(w, "missing bucket or keys. usage: /reduce?bucket=YOUR_BUCKET&keys=key1,key2,key3", http.StatusBadRequest)
			return
		}

		mapperKeys := strings.Split(keysParam, ",")
		if len(mapperKeys) < 1 {
			http.Error(w, "no mapper keys provided", http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			http.Error(w, "aws config error: "+err.Error(), 500)
			return
		}
		s3c := s3.NewFromConfig(awsCfg)

		finalCounts := make(map[string]int)
		inputS3s := make([]string, 0, len(mapperKeys))

		for _, k := range mapperKeys {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			inputS3s = append(inputS3s, fmt.Sprintf("s3://%s/%s", bucket, k))

			obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{
				Bucket: &bucket,
				Key:    &k,
			})
			if err != nil {
				http.Error(w, "getobject error for "+k+": "+err.Error(), 500)
				return
			}

			data, err := io.ReadAll(obj.Body)
			obj.Body.Close()
			if err != nil {
				http.Error(w, "read mapper body error for "+k+": "+err.Error(), 500)
				return
			}

			counts, err := decodeCountsFlexible(data)
			if err != nil {
				http.Error(w, "decode mapper json error for "+k+": "+err.Error(), 500)
				return
			}

			for word, c := range counts {
				finalCounts[word] += c
			}
		}

		// Build top10
		type pair struct {
			word  string
			count int
		}
		pairs := make([]pair, 0, len(finalCounts))
		total := 0
		for w2, c2 := range finalCounts {
			pairs = append(pairs, pair{w2, c2})
			total += c2
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].count == pairs[j].count {
				return pairs[i].word < pairs[j].word
			}
			return pairs[i].count > pairs[j].count
		})

		topN := 10
		if len(pairs) < topN {
			topN = len(pairs)
		}
		top10 := make([]MapperKV, 0, topN)
		for i := 0; i < topN; i++ {
			top10 = append(top10, MapperKV{Word: pairs[i].word, Count: pairs[i].count})
		}

		reduceRunID := fmt.Sprintf("reduce-%d", time.Now().Unix())
		outKey := fmt.Sprintf("results/%s/final.json", reduceRunID)

		outObj := map[string]any{
			"input_mapper_s3": inputS3s,
			"run_id":          reduceRunID,
			"total_words":     total,
			"unique_words":    len(finalCounts),
			"counts":          finalCounts,
			"top10":           top10,
		}
		outBytes, _ := json.MarshalIndent(outObj, "", "  ")

		_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      &bucket,
			Key:         &outKey,
			Body:        strings.NewReader(string(outBytes)),
			ContentType: strPtr("application/json"),
		})
		if err != nil {
			http.Error(w, "putobject error: "+err.Error(), 500)
			return
		}

		resp := ReduceResponse{
			InputMapperS3: inputS3s,
			OutputS3:      fmt.Sprintf("s3://%s/%s", bucket, outKey),
			OutputKey:     outKey,
			RunID:         reduceRunID,
			Top10:         top10,
			TotalWords:    total,
			UniqueWords:   len(finalCounts),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	log.Printf("reducer listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func strPtr(s string) *string { return &s }
