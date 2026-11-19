// Command supporteval runs support.Prepare on JSONL conversations (stdin) with the OpenAI key in
// OPENAI_API_KEY and writes one JSONL result per case (stdout). It never sends WhatsApp messages.
// Evaluation tool only; not part of the server.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shridarpatil/whatomate/internal/builtin"
	"github.com/shridarpatil/whatomate/internal/support"
)

type evalCase struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	FollowUp bool              `json:"follow_up"`
	Messages []support.Message `json:"messages"`
	OldReply string            `json:"old_reply"`
}

var reasoning = regexp.MustCompile(`^(gpt-([5-9]|[1-9][0-9])|o[0-9])`)

const schema = `{"type":"object","additionalProperties":false,"required":["decision","reason","language","summary","questions"],"properties":{"decision":{"type":"string","enum":["answer","skip","email"]},"reason":{"type":"string"},"language":{"type":"string","enum":["en","roman_ur","ur"]},"summary":{"type":"string"},"questions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["category","question","message_ids","attempted_steps"],"properties":{"category":{"type":"string","enum":["account","web","portfolio","market_data","subscription","ads","brokerage","feedback","other"]},"question":{"type":"string"},"message_ids":{"type":"array","items":{"type":"string"}},"attempted_steps":{"type":"array","items":{"type":"string"}}}}}}}`

func generator(key, model string) support.Generate {
	client := &http.Client{Timeout: 90 * time.Second}
	return func(ctx context.Context, system, input string) (string, error) {
		payload := map[string]any{"model": model, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": input}}}
		if reasoning.MatchString(model) {
			payload["max_completion_tokens"] = 4000
			payload["reasoning_effort"] = "low"
		} else {
			payload["max_tokens"] = 4000
			payload["temperature"] = 0.2
		}
		if system == builtin.InvestifyQueuePrompt {
			payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "support_decision", "strict": true, "schema": json.RawMessage(schema)}}
		}
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256000))
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		var result struct {
			Choices []struct {
				Message      struct{ Content string } `json:"message"`
				FinishReason string                   `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" {
			return "", errors.New("incomplete response")
		}
		return result.Choices[0].Message.Content, nil
	}
}

func main() {
	key, model := os.Getenv("OPENAI_API_KEY"), os.Getenv("MODEL")
	if key == "" || model == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY and MODEL are required")
		os.Exit(2)
	}
	gen := generator(key, model)
	var cases []evalCase
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			var c evalCase
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				fmt.Fprintln(os.Stderr, "bad case:", err)
				continue
			}
			cases = append(cases, c)
		}
	}
	out := make([]map[string]any, len(cases))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c evalCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			start := time.Now()
			p, err := support.Prepare(ctx, c.Messages, gen, support.Options{FollowUp: c.FollowUp})
			r := map[string]any{"id": c.ID, "kind": c.Kind, "follow_up": c.FollowUp, "seconds": time.Since(start).Seconds(), "old_reply": c.OldReply}
			if err != nil {
				r["error"] = err.Error()
			} else {
				r["decision"], r["language"], r["questions"], r["body"], r["fallback"] = p.Decision.Decision, p.Language, p.Questions, p.Body, p.Fallback
			}
			out[i] = r
		}(i, c)
	}
	wg.Wait()
	enc := json.NewEncoder(os.Stdout)
	for _, r := range out {
		_ = enc.Encode(r)
	}
}
