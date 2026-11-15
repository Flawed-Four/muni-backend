package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Flawed-Four/muni-backend/internal/config"
	"github.com/Flawed-Four/muni-backend/internal/domain"
	"github.com/Flawed-Four/muni-backend/internal/infrastructure/llm"
)

// Gemini is a paid API that cannot run in compose, so live tests skip without a key.
func requireGeminiKey(t *testing.T) {
	t.Helper()
	if testCfg.Gemini.APIKey == "" {
		t.Skip("TEST_GEMINI_API_KEY is not set; skipping live Gemini test")
	}
}

func newLLM(t *testing.T) *llm.Gemini {
	t.Helper()
	requireGeminiKey(t)
	g, cleanup, err := llm.NewGemini(testCfg)
	if err != nil {
		t.Fatalf("NewGemini: %v", err)
	}
	t.Cleanup(cleanup)
	return g
}

func generate(t *testing.T, g *llm.Gemini, req domain.LLMRequest) *domain.LLMResponse {
	t.Helper()
	resp, err := g.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return resp
}

func text(s string) []domain.LLMPart {
	return []domain.LLMPart{{Text: s}}
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

var weatherTool = domain.LLMTool{
	Name:        "get_weather",
	Description: "Returns the current temperature in a city.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string", "description": "City name"},
		},
		"required": []string{"city"},
	},
}

func TestGemini_RejectsInvalidConfig(t *testing.T) {
	valid := config.GeminiConfig{
		APIKey:        "test-key",
		Model:         "gemini-3.5-flash-lite",
		ThinkingLevel: "low",
		Timeout:       time.Second,
		MaxRetries:    1,
	}
	tests := []struct {
		name   string
		change func(*config.GeminiConfig)
		want   string
	}{
		{"no api key", func(c *config.GeminiConfig) { c.APIKey = "" }, "api key is not set"},
		{"no model", func(c *config.GeminiConfig) { c.Model = "" }, "model is not set"},
		{"unknown thinking level", func(c *config.GeminiConfig) { c.ThinkingLevel = "max" }, "thinking level"},
		{"zero timeout", func(c *config.GeminiConfig) { c.Timeout = 0 }, "timeout must be positive"},
		{"negative retries", func(c *config.GeminiConfig) { c.MaxRetries = -1 }, "retries must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := *testCfg
			cfg.Gemini = valid
			tt.change(&cfg.Gemini)

			_, _, err := llm.NewGemini(&cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewGemini error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// Tests our timeout against a server that never answers, not Gemini itself.
func TestGemini_TimeoutBoundsStalledServer(t *testing.T) {
	addr := stalledServer(t)

	const timeout = 300 * time.Millisecond
	cfg := *testCfg
	cfg.Gemini = config.GeminiConfig{
		APIKey:        "test-key",
		Model:         "gemini-3.5-flash-lite",
		ThinkingLevel: "low",
		Timeout:       timeout,
		BaseURL:       "http://" + addr.String(),
	}

	start := time.Now()
	_, _, err := llm.NewGemini(&cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("NewGemini succeeded against a server that never answers")
	}
	if elapsed > 10*timeout {
		t.Fatalf("NewGemini returned after %s, want it bounded by the %s timeout", elapsed, timeout)
	}
}

func TestGemini_InvalidAPIKey(t *testing.T) {
	requireGeminiKey(t)
	cfg := *testCfg
	cfg.Gemini.APIKey = "invalid-key"

	if _, _, err := llm.NewGemini(&cfg); err == nil {
		t.Fatal("NewGemini accepted an invalid API key")
	}
}

func TestGemini_UnknownModel(t *testing.T) {
	g := newLLM(t)

	cfg := *testCfg
	cfg.Gemini.Model = "gemini-does-not-exist"
	if _, _, err := llm.NewGemini(&cfg); err == nil {
		t.Fatal("NewGemini accepted an unknown model")
	}

	_, err := g.Generate(context.Background(), domain.LLMRequest{
		Model: "gemini-does-not-exist",
		Input: text("Hello"),
	})
	if err == nil {
		t.Fatal("Generate accepted an unknown model")
	}
}

func TestGemini_Text(t *testing.T) {
	g := newLLM(t)

	resp := generate(t, g, domain.LLMRequest{Input: text("Reply with exactly one word: pong")})

	if !containsFold(resp.Text, "pong") {
		t.Fatalf("text = %q, want it to contain pong", resp.Text)
	}
	if resp.ID == "" {
		t.Fatal("response has no ID")
	}
	if len(resp.FunctionCalls) != 0 {
		t.Fatalf("unexpected function calls: %+v", resp.FunctionCalls)
	}
	if resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 || resp.Usage.TotalTokens == 0 {
		t.Fatalf("usage = %+v, want token counts", resp.Usage)
	}
}

func TestGemini_SystemInstruction(t *testing.T) {
	g := newLLM(t)

	resp := generate(t, g, domain.LLMRequest{
		System: "Whatever the user says, reply with exactly the word: banana",
		Input:  text("Hello there!"),
	})

	if !containsFold(resp.Text, "banana") {
		t.Fatalf("text = %q, want it to follow the system instruction", resp.Text)
	}
}

func TestGemini_ThinkingOverride(t *testing.T) {
	g := newLLM(t)

	for _, level := range []domain.ThinkingLevel{domain.ThinkingMinimal, domain.ThinkingHigh} {
		resp := generate(t, g, domain.LLMRequest{Input: text("What is 2 + 2? Reply with the number only."), Thinking: level})
		if !strings.Contains(resp.Text, "4") {
			t.Fatalf("thinking %s: text = %q, want 4", level, resp.Text)
		}
	}
}

func TestGemini_MultiTurn(t *testing.T) {
	g := newLLM(t)

	first := generate(t, g, domain.LLMRequest{Input: text("My favourite colour is teal. Reply with OK.")})
	second := generate(t, g, domain.LLMRequest{
		PreviousID: first.ID,
		Input:      text("What is my favourite colour? Answer with one word."),
	})

	if !containsFold(second.Text, "teal") {
		t.Fatalf("text = %q, want the model to remember teal", second.Text)
	}
	if second.ID == first.ID {
		t.Fatal("second turn reused the first ID")
	}
}

func TestGemini_StructuredOutput(t *testing.T) {
	g := newLLM(t)

	resp := generate(t, g, domain.LLMRequest{
		Input: text("Which city and country is the Eiffel Tower in?"),
		ResponseSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city":    map[string]any{"type": "string"},
				"country": map[string]any{"type": "string"},
			},
			"required": []string{"city", "country"},
		},
	})

	var got struct {
		City    string `json:"city"`
		Country string `json:"country"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &got); err != nil {
		t.Fatalf("text is not JSON: %v\n%s", err, resp.Text)
	}
	if !containsFold(got.City, "paris") || !containsFold(got.Country, "france") {
		t.Fatalf("got %+v, want Paris, France", got)
	}
}

// askWeather makes the model call get_weather for Jakarta and returns that call.
func askWeather(t *testing.T, g *llm.Gemini, req domain.LLMRequest) (*domain.LLMResponse, domain.LLMFunctionCall) {
	t.Helper()
	req.Input = text("What is the temperature in Jakarta right now?")
	req.Tools = []domain.LLMTool{weatherTool}

	resp := generate(t, g, req)
	if len(resp.FunctionCalls) == 0 {
		t.Fatalf("no function call, text = %q", resp.Text)
	}
	call := resp.FunctionCalls[0]
	if call.Name != "get_weather" || call.ID == "" {
		t.Fatalf("call = %+v, want get_weather with an ID", call)
	}
	if city, _ := call.Args["city"].(string); !containsFold(city, "jakarta") {
		t.Fatalf("args = %v, want city Jakarta", call.Args)
	}
	return resp, call
}

func TestGemini_FunctionCalling(t *testing.T) {
	g := newLLM(t)

	first, call := askWeather(t, g, domain.LLMRequest{})
	final := generate(t, g, domain.LLMRequest{
		PreviousID: first.ID,
		Tools:      []domain.LLMTool{weatherTool},
		FunctionResults: []domain.LLMFunctionResult{{
			CallID: call.ID,
			Name:   call.Name,
			Result: map[string]any{"temperature_celsius": 37},
		}},
	})

	if len(final.FunctionCalls) != 0 {
		t.Fatalf("unexpected function calls: %+v", final.FunctionCalls)
	}
	if !strings.Contains(final.Text, "37") {
		t.Fatalf("text = %q, want it to use the function result 37", final.Text)
	}
}

func TestGemini_FunctionCallingWithStructuredOutput(t *testing.T) {
	g := newLLM(t)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city":                map[string]any{"type": "string"},
			"temperature_celsius": map[string]any{"type": "number"},
		},
		"required": []string{"city", "temperature_celsius"},
	}

	first, call := askWeather(t, g, domain.LLMRequest{ResponseSchema: schema})
	final := generate(t, g, domain.LLMRequest{
		PreviousID:     first.ID,
		Tools:          []domain.LLMTool{weatherTool},
		ResponseSchema: schema,
		FunctionResults: []domain.LLMFunctionResult{{
			CallID: call.ID,
			Name:   call.Name,
			Result: map[string]any{"temperature_celsius": 37},
		}},
	})

	var got struct {
		City        string  `json:"city"`
		Temperature float64 `json:"temperature_celsius"`
	}
	if err := json.Unmarshal([]byte(final.Text), &got); err != nil {
		t.Fatalf("text is not JSON: %v\n%s", err, final.Text)
	}
	if !containsFold(got.City, "jakarta") || got.Temperature != 37 {
		t.Fatalf("got %+v, want Jakarta at 37", got)
	}
}

func TestGemini_ToolChoice(t *testing.T) {
	g := newLLM(t)

	forced := generate(t, g, domain.LLMRequest{
		Input:      text("Say hello."),
		Tools:      []domain.LLMTool{weatherTool},
		ToolChoice: domain.ToolChoiceAny,
	})
	if len(forced.FunctionCalls) == 0 {
		t.Fatalf("tool choice any: no function call, text = %q", forced.Text)
	}

	none := generate(t, g, domain.LLMRequest{
		Input:      text("What is the temperature in Jakarta right now?"),
		Tools:      []domain.LLMTool{weatherTool},
		ToolChoice: domain.ToolChoiceNone,
	})
	if len(none.FunctionCalls) != 0 || none.Text == "" {
		t.Fatalf("tool choice none: calls = %+v, text = %q, want text only", none.FunctionCalls, none.Text)
	}
}

// sineWAV builds a mono 16-bit PCM WAV file with a plain tone.
func sineWAV(seconds, sampleRate int, freq float64) []byte {
	samples := seconds * sampleRate
	dataSize := uint32(samples * 2)

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, 36+dataSize)
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(&buf, binary.LittleEndian, []any{
		uint32(16), uint16(1), uint16(1), // fmt size, PCM, mono
		uint32(sampleRate), uint32(sampleRate * 2), uint16(2), uint16(16),
	})
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	for i := range samples {
		v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		_ = binary.Write(&buf, binary.LittleEndian, int16(v*0.3*math.MaxInt16))
	}
	return buf.Bytes()
}

func TestGemini_Audio(t *testing.T) {
	g := newLLM(t)

	resp := generate(t, g, domain.LLMRequest{
		Input: []domain.LLMPart{
			{Text: "Describe this audio in one short sentence."},
			// audio/x-wav also checks that MIME aliases are mapped.
			{Audio: &domain.LLMAudio{Data: sineWAV(2, 16000, 440), MIMEType: "audio/x-wav"}},
		},
	})

	if resp.Text == "" {
		t.Fatal("empty text for audio input")
	}
	if resp.Usage.AudioInputTokens == 0 {
		t.Fatalf("usage = %+v, want audio input tokens", resp.Usage)
	}
}

func TestGemini_RejectsInvalidRequest(t *testing.T) {
	g := newLLM(t)
	audio := func(data []byte, mimeType string) []domain.LLMPart {
		return []domain.LLMPart{{Audio: &domain.LLMAudio{Data: data, MIMEType: mimeType}}}
	}
	result := []domain.LLMFunctionResult{{CallID: "call-1", Result: "ok"}}

	tests := []struct {
		name string
		req  domain.LLMRequest
		want string
	}{
		{"no input", domain.LLMRequest{}, "no input"},
		{"empty part", domain.LLMRequest{Input: []domain.LLMPart{{}}}, "is empty"},
		{"text and audio in one part", domain.LLMRequest{Input: []domain.LLMPart{{Text: "hi", Audio: &domain.LLMAudio{Data: []byte{1}, MIMEType: "audio/wav"}}}}, "both text and audio"},
		{"video mime type", domain.LLMRequest{Input: audio([]byte{1}, "video/mp4")}, "unsupported audio mime type"},
		{"broken mime type", domain.LLMRequest{Input: audio([]byte{1}, "")}, "invalid audio mime type"},
		{"empty audio", domain.LLMRequest{Input: audio(nil, "audio/wav")}, "empty audio"},
		{"audio too large", domain.LLMRequest{Input: audio(make([]byte, 14<<20+1), "audio/wav")}, "larger than"},
		{"results without previous id", domain.LLMRequest{FunctionResults: result}, "previous id"},
		{"result without call id", domain.LLMRequest{PreviousID: "x", FunctionResults: []domain.LLMFunctionResult{{Result: "ok"}}}, "no call id"},
		{"unknown tool choice", domain.LLMRequest{Input: text("hi"), ToolChoice: "sometimes"}, "unknown tool choice"},
		{"unknown thinking level", domain.LLMRequest{Input: text("hi"), Thinking: "max"}, "unknown thinking level"},
		{"tool without name", domain.LLMRequest{Input: text("hi"), Tools: []domain.LLMTool{{}}}, "has no name"},
		{"duplicate tool", domain.LLMRequest{Input: text("hi"), Tools: []domain.LLMTool{weatherTool, weatherTool}}, "declared twice"},
		{"negative max output tokens", domain.LLMRequest{Input: text("hi"), MaxOutputTokens: -1}, "must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := g.Generate(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Generate error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
