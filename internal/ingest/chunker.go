package ingest

import "strings"

const (
	TargetChunkChars  = 2000
	OverlapChars      = 200
	MaxToolOutputSize = 2048
)

func ChunkText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= TargetChunkChars {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < len(text) {
		end := start + TargetChunkChars
		if end >= len(text) {
			chunks = append(chunks, text[start:])
			break
		}
		cut := end
		for i := end; i > start+TargetChunkChars/2 && i < len(text); i-- {
			if text[i] == '\n' {
				cut = i
				break
			}
		}
		chunks = append(chunks, text[start:cut])
		start = cut - OverlapChars
		if start < 0 {
			start = 0
		}
		if cut == start {
			break
		}
	}
	return chunks
}
