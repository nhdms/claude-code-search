package transcript

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

type RawLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	Version     string          `json:"version"`
	GitBranch   string          `json:"gitBranch"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

type Message struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type ContentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

type Event struct {
	UUID       string
	ParentUUID string
	SessionID  string
	Timestamp  string
	CWD        string
	Role       string
	Model      string
	Text       string
	ToolName   string
	ToolInput  string
	ToolOutput string
	Kind       string
}

func ParseFile(path string, offset int64, emit func(ev Event, newOffset int64) error) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return offset, err
		}
	}
	r := bufio.NewReaderSize(f, 1<<20)
	cur := offset
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			cur += int64(len(line))
			trimmed := strings.TrimSpace(string(line))
			if trimmed == "" {
				continue
			}
			var raw RawLine
			if jerr := json.Unmarshal([]byte(trimmed), &raw); jerr != nil {
				continue
			}
			evs := extractEvents(raw)
			for _, ev := range evs {
				if cerr := emit(ev, cur); cerr != nil {
					return cur, cerr
				}
			}
		}
		if err == io.EOF {
			return cur, nil
		}
		if err != nil {
			return cur, err
		}
	}
}

func extractEvents(raw RawLine) []Event {
	if raw.Type != "user" && raw.Type != "assistant" {
		return nil
	}
	var msg Message
	if len(raw.Message) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw.Message, &msg); err != nil {
		return nil
	}

	base := Event{
		UUID:       raw.UUID,
		ParentUUID: raw.ParentUUID,
		SessionID:  raw.SessionID,
		Timestamp:  raw.Timestamp,
		CWD:        raw.CWD,
		Role:       msg.Role,
		Model:      msg.Model,
	}

	if len(msg.Content) == 0 {
		return nil
	}
	if msg.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(msg.Content, &s); err == nil {
			ev := base
			ev.Text = s
			ev.Kind = "text"
			return []Event{ev}
		}
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}
	out := make([]Event, 0, len(blocks))
	for _, b := range blocks {
		ev := base
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			ev.Text = b.Text
			ev.Kind = "text"
		case "thinking":
			continue
		case "tool_use":
			ev.ToolName = b.Name
			ev.ToolInput = string(b.Input)
			ev.Kind = "tool_use"
		case "tool_result":
			ev.ToolOutput = contentToString(b.Content)
			if strings.TrimSpace(ev.ToolOutput) == "" {
				continue
			}
			ev.Kind = "tool_result"
		default:
			continue
		}
		out = append(out, ev)
	}
	return out
}

func contentToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				sb.WriteString(b.Text)
				sb.WriteByte('\n')
			}
		}
		return sb.String()
	}
	return string(raw)
}
