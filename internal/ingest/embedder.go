package ingest

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/sashabaranov/go-openai"
)

type Embedder struct {
	Client *openai.Client
	Model  string
	Dim    int
	Batch  int
}

func NewEmbedder(apiKey, model string, dim int) *Embedder {
	return NewEmbedderWithBase(apiKey, model, dim, "")
}

func NewEmbedderWithBase(apiKey, model string, dim int, baseURL string) *Embedder {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &Embedder{
		Client: openai.NewClientWithConfig(cfg),
		Model:  model,
		Dim:    dim,
		Batch:  96,
	}
}

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for i := 0; i < len(texts); i += e.Batch {
		end := i + e.Batch
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		var resp openai.EmbeddingResponse
		var err error
		for attempt := 0; attempt < 5; attempt++ {
			resp, err = e.Client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
				Input: batch,
				Model: openai.EmbeddingModel(e.Model),
			})
			if err == nil {
				break
			}
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
		if err != nil {
			return nil, fmt.Errorf("openai embed: %w", err)
		}
		if len(resp.Data) != len(batch) {
			return nil, fmt.Errorf("embed count mismatch: got %d want %d", len(resp.Data), len(batch))
		}
		for _, d := range resp.Data {
			if len(d.Embedding) != e.Dim {
				return nil, fmt.Errorf("embed dim %d != %d", len(d.Embedding), e.Dim)
			}
			out = append(out, d.Embedding)
		}
	}
	return out, nil
}

func Float32SliceToBlob(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}
