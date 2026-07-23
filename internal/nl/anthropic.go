package nl

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicCompleter calls the Claude Messages API. It satisfies Completer.
type AnthropicCompleter struct {
	client    anthropic.Client
	model     anthropic.Model
	maxTokens int64
}

// AnthropicConfig configures the completer.
type AnthropicConfig struct {
	APIKey    string // optional; falls back to ANTHROPIC_API_KEY
	Model     string // optional; defaults to claude-opus-4-8
	MaxTokens int64  // optional; defaults to 16000
}

// NewAnthropicCompleter builds a completer. With no APIKey it uses the standard
// credential resolution (ANTHROPIC_API_KEY, then any configured profile).
func NewAnthropicCompleter(cfg AnthropicConfig) *AnthropicCompleter {
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	model := anthropic.ModelClaudeOpus4_8
	if cfg.Model != "" {
		model = anthropic.Model(cfg.Model)
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16000
	}
	return &AnthropicCompleter{
		client:    anthropic.NewClient(opts...),
		model:     model,
		maxTokens: maxTokens,
	}
}

// Complete sends the system prompt and conversation to Claude and returns the
// concatenated text of the response. Adaptive thinking is enabled — the model
// decides how much to reason about the layout — and the system prompt is marked
// cacheable so the schema/catalogue prefix is reused across retries.
func (a *AnthropicCompleter) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	params := anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: a.maxTokens,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: toAnthropicMessages(messages),
	}

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("model refused the request (category: %s)", resp.StopDetails.Category)
	}

	var out strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(t.Text)
		}
	}
	return out.String(), nil
}

func toAnthropicMessages(messages []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(messages))
	for _, m := range messages {
		block := anthropic.NewTextBlock(m.Text)
		if m.Role == "assistant" {
			out = append(out, anthropic.NewAssistantMessage(block))
		} else {
			out = append(out, anthropic.NewUserMessage(block))
		}
	}
	return out
}
