// Package llm calls Gemini through the Interactions API.
package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"google.golang.org/genai"
	"google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
	"google.golang.org/genai/interactions/retry"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
)

// Gemini caps a request at 20 MB and base64 makes audio about 4/3 bigger.
const maxInlineAudioBytes = 14 << 20

// Other names browsers and phones use for the same formats.
var audioMIMEAliases = map[string]string{
	"audio/x-wav":  "audio/wav",
	"audio/wave":   "audio/wav",
	"audio/x-m4a":  "audio/m4a",
	"audio/mp4":    "audio/m4a",
	"audio/x-aiff": "audio/aiff",
	"audio/x-flac": "audio/flac",
}

// Gemini implements domain.LanguageModel and is safe for concurrent use.
type Gemini struct {
	client   *genai.Client
	model    string
	thinking domain.ThinkingLevel
	retries  operations.Option
}

// NewGemini fails if the API key is rejected or the model does not exist.
func NewGemini(cfg *config.Config) (*Gemini, func(), error) {
	c := cfg.Gemini
	thinking := domain.ThinkingLevel(c.ThinkingLevel)

	switch {
	// Without a key the SDK would silently read one from the environment.
	case c.APIKey == "":
		return nil, nil, errors.New("gemini api key is not set")
	case c.Model == "":
		return nil, nil, errors.New("gemini model is not set")
	case !validThinking(thinking):
		return nil, nil, fmt.Errorf("gemini thinking level %q is not one of minimal, low, medium, high", c.ThinkingLevel)
	// A zero timeout means "never time out" to http.Client.
	case c.Timeout <= 0:
		return nil, nil, fmt.Errorf("gemini timeout must be positive, got %s", c.Timeout)
	case c.MaxRetries < 0:
		return nil, nil, fmt.Errorf("gemini max retries must not be negative, got %d", c.MaxRetries)
	}

	httpClient := &http.Client{
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
		Timeout:   c.Timeout,
		// Don't follow redirects: they would send the API key to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	cleanup := httpClient.CloseIdleConnections

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      c.APIKey,
		Backend:     genai.BackendGeminiAPI,
		HTTPClient:  httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: c.BaseURL},
	})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create gemini client: %w", err)
	}
	if _, err := client.Models.Get(ctx, c.Model, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("check gemini model %q: %w", c.Model, err)
	}

	return &Gemini{
		client:   client,
		model:    c.Model,
		thinking: thinking,
		retries:  operations.WithRetries(retryConfig(c.MaxRetries)),
	}, cleanup, nil
}

// retryConfig keeps the SDK's default backoff but uses our retry count.
func retryConfig(maxRetries int) retry.Config {
	if maxRetries == 0 {
		return retry.Config{Strategy: "none"}
	}
	return retry.Config{
		Strategy: "attempt-count-backoff",
		Backoff: &retry.BackoffStrategy{
			InitialInterval: 500,
			MaxInterval:     8000,
			Exponent:        2,
			MaxElapsedTime:  30000,
		},
		RetryConnectionErrors: true,
		MaxRetries:            &maxRetries,
	}
}

func (g *Gemini) Generate(ctx context.Context, req domain.LLMRequest) (*domain.LLMResponse, error) {
	body, err := g.buildRequest(req)
	if err != nil {
		return nil, fmt.Errorf("gemini generate: %w", err)
	}

	res, err := g.client.Interactions.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(body),
	}, g.retries)
	if err != nil {
		return nil, fmt.Errorf("gemini generate: %w", err)
	}
	if res.Interaction == nil {
		return nil, errors.New("gemini generate: empty response")
	}

	out, err := toResponse(res.Interaction)
	if err != nil {
		return nil, fmt.Errorf("gemini generate: %w", err)
	}
	return out, nil
}

func (g *Gemini) buildRequest(req domain.LLMRequest) (interactions.CreateModelInteraction, error) {
	var body interactions.CreateModelInteraction

	if len(req.Input) == 0 && len(req.FunctionResults) == 0 {
		return body, errors.New("request has no input and no function results")
	}
	if len(req.FunctionResults) > 0 && req.PreviousID == "" {
		return body, errors.New("function results need the previous id")
	}
	if !validToolChoice(req.ToolChoice) {
		return body, fmt.Errorf("unknown tool choice %q", req.ToolChoice)
	}
	if req.MaxOutputTokens < 0 {
		return body, fmt.Errorf("max output tokens must not be negative, got %d", req.MaxOutputTokens)
	}

	thinking := req.Thinking
	if thinking == "" {
		thinking = g.thinking
	}
	if !validThinking(thinking) {
		return body, fmt.Errorf("unknown thinking level %q", thinking)
	}

	model := req.Model
	if model == "" {
		model = g.model
	}

	input, err := toInput(req.Input, req.FunctionResults)
	if err != nil {
		return body, err
	}
	tools, err := toTools(req.Tools)
	if err != nil {
		return body, err
	}

	body = interactions.CreateModelInteraction{
		Model: interactions.Model(model),
		Input: &input,
		Store: new(true),
		Tools: tools,
		GenerationConfig: &interactions.GenerationConfig{
			ThinkingLevel: interactions.ThinkingLevel(thinking).ToPointer(),
		},
	}
	if req.PreviousID != "" {
		body.PreviousInteractionID = new(req.PreviousID)
	}
	if req.System != "" {
		body.SystemInstruction = new(req.System)
	}
	if req.MaxOutputTokens > 0 {
		body.GenerationConfig.MaxOutputTokens = new(req.MaxOutputTokens)
	}
	if req.ToolChoice != "" {
		choice := interactions.NewToolChoice(interactions.ToolChoiceType(req.ToolChoice))
		body.GenerationConfig.ToolChoice = &choice
	}
	if req.ResponseSchema != nil {
		format := interactions.NewCreateModelInteractionResponseFormat(interactions.NewResponseFormat(interactions.TextResponseFormat{
			MimeType: interactions.TextResponseFormatMimeTypeApplicationJSON.ToPointer(),
			Schema:   req.ResponseSchema,
		}))
		body.ResponseFormat = &format
	}
	return body, nil
}

func toInput(parts []domain.LLMPart, results []domain.LLMFunctionResult) (interactions.InteractionsInput, error) {
	contents, err := toContents(parts)
	if err != nil {
		return interactions.InteractionsInput{}, err
	}
	if len(results) == 0 {
		return interactions.NewInteractionsInput(contents), nil
	}

	steps := make([]interactions.Step, 0, len(results)+1)
	for i, r := range results {
		if r.CallID == "" {
			return interactions.InteractionsInput{}, fmt.Errorf("function result %d has no call id", i)
		}
		data, err := json.Marshal(r.Result)
		if err != nil {
			return interactions.InteractionsInput{}, fmt.Errorf("function result %d: %w", i, err)
		}
		step := interactions.FunctionResultStep{
			CallID: r.CallID,
			// The SDK's object form drops every field, so the result goes as JSON text.
			Result: interactions.NewFunctionResultStepResultUnion(string(data)),
		}
		if r.Name != "" {
			step.Name = new(r.Name)
		}
		if r.IsError {
			step.IsError = new(true)
		}
		steps = append(steps, interactions.NewStep(step))
	}
	if len(contents) > 0 {
		steps = append(steps, interactions.NewStep(interactions.UserInputStep{Content: contents}))
	}
	return interactions.NewInteractionsInput(steps), nil
}

func toContents(parts []domain.LLMPart) ([]interactions.Content, error) {
	contents := make([]interactions.Content, 0, len(parts))
	audioBytes := 0

	for i, p := range parts {
		switch {
		case p.Audio != nil && p.Text != "":
			return nil, fmt.Errorf("part %d has both text and audio", i)
		case p.Audio != nil:
			mimeType, err := audioMIMEType(p.Audio.MIMEType)
			if err != nil {
				return nil, fmt.Errorf("part %d: %w", i, err)
			}
			if len(p.Audio.Data) == 0 {
				return nil, fmt.Errorf("part %d has empty audio", i)
			}
			audioBytes += len(p.Audio.Data)
			if audioBytes > maxInlineAudioBytes {
				return nil, fmt.Errorf("audio is larger than %d bytes", maxInlineAudioBytes)
			}
			contents = append(contents, interactions.NewContent(interactions.AudioContent{
				Data:     new(base64.StdEncoding.EncodeToString(p.Audio.Data)),
				MimeType: mimeType.ToPointer(),
			}))
		case p.Text != "":
			contents = append(contents, interactions.NewContent(interactions.TextContent{Text: p.Text}))
		default:
			return nil, fmt.Errorf("part %d is empty", i)
		}
	}
	return contents, nil
}

// audioMIMEType turns e.g. "audio/webm;codecs=opus" into "audio/webm".
func audioMIMEType(s string) (interactions.AudioContentMimeType, error) {
	mediaType, _, err := mime.ParseMediaType(s)
	if err != nil {
		return "", fmt.Errorf("invalid audio mime type %q: %w", s, err)
	}
	if alias, ok := audioMIMEAliases[mediaType]; ok {
		mediaType = alias
	}
	t := interactions.AudioContentMimeType(mediaType)
	if !t.IsExact() {
		return "", fmt.Errorf("unsupported audio mime type %q", s)
	}
	return t, nil
}

func toTools(tools []domain.LLMTool) ([]interactions.Tool, error) {
	out := make([]interactions.Tool, 0, len(tools))
	seen := make(map[string]bool, len(tools))

	for i, t := range tools {
		if t.Name == "" {
			return nil, fmt.Errorf("tool %d has no name", i)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("tool %q is declared twice", t.Name)
		}
		seen[t.Name] = true

		fn := interactions.Function{Name: new(t.Name)}
		if t.Description != "" {
			fn.Description = new(t.Description)
		}
		if t.Parameters != nil {
			fn.Parameters = t.Parameters
		}
		out = append(out, interactions.NewTool(fn))
	}
	return out, nil
}

func toResponse(in *interactions.Interaction) (*domain.LLMResponse, error) {
	if in.Status != interactions.InteractionStatusCompleted && in.Status != interactions.InteractionStatusRequiresAction {
		return nil, fmt.Errorf("interaction ended with status %q%s", in.Status, errorDetail(in.Errors))
	}

	out := &domain.LLMResponse{
		ID:    deref(in.ID),
		Text:  deref(in.OutputText),
		Usage: toUsage(in.Usage),
	}
	if in.Model != nil {
		out.Model = string(*in.Model)
	}

	for _, step := range in.Steps {
		if s := step.ModelOutputStep; s != nil && s.Error != nil {
			return nil, fmt.Errorf("model output failed: %s", deref(s.Error.Message))
		}
		if s := step.FunctionCallStep; s != nil {
			out.FunctionCalls = append(out.FunctionCalls, domain.LLMFunctionCall{
				ID:   s.ID,
				Name: s.Name,
				Args: s.Arguments,
			})
		}
	}
	return out, nil
}

func toUsage(u *interactions.Usage) domain.LLMUsage {
	if u == nil {
		return domain.LLMUsage{}
	}
	usage := domain.LLMUsage{
		InputTokens:   deref(u.TotalInputTokens),
		OutputTokens:  deref(u.TotalOutputTokens),
		ThoughtTokens: deref(u.TotalThoughtTokens),
		TotalTokens:   deref(u.TotalTokens),
	}
	for _, m := range u.InputTokensByModality {
		if m.Modality != nil && *m.Modality == interactions.ResponseModalityAudio {
			usage.AudioInputTokens += deref(m.Tokens)
		}
	}
	return usage
}

func errorDetail(errs []interactions.Error) string {
	if len(errs) == 0 {
		return ""
	}
	return ": " + deref(errs[0].Message)
}

func validThinking(l domain.ThinkingLevel) bool {
	switch l {
	case domain.ThinkingMinimal, domain.ThinkingLow, domain.ThinkingMedium, domain.ThinkingHigh:
		return true
	}
	return false
}

func validToolChoice(c domain.ToolChoice) bool {
	switch c {
	case "", domain.ToolChoiceAuto, domain.ToolChoiceAny, domain.ToolChoiceNone, domain.ToolChoiceValidated:
		return true
	}
	return false
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
