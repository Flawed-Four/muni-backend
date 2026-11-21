package domain

import "context"

type ThinkingLevel string

const (
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)

type ToolChoice string

const (
	ToolChoiceAuto      ToolChoice = "auto" // the model decides
	ToolChoiceAny       ToolChoice = "any"  // the model must call a tool
	ToolChoiceNone      ToolChoice = "none"
	ToolChoiceValidated ToolChoice = "validated" // tool calls must match the schema
)

type LLMAudio struct {
	Data     []byte
	MIMEType string // e.g. "audio/webm"
}

// LLMPart holds either Text or Audio, not both.
type LLMPart struct {
	Text  string
	Audio *LLMAudio
}

type LLMTool struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema of the arguments; nil = no arguments
}

type LLMFunctionCall struct {
	ID   string
	Name string
	Args map[string]any
}

type LLMFunctionResult struct {
	CallID  string // LLMFunctionCall.ID
	Name    string
	Result  any // anything json.Marshal accepts
	IsError bool
}

type LLMRequest struct {
	Model           string // "" = default model from config
	PreviousID      string // LLMResponse.ID of the previous turn; "" = new conversation
	System          string
	Input           []LLMPart
	FunctionResults []LLMFunctionResult // needs PreviousID
	Tools           []LLMTool
	ToolChoice      ToolChoice     // "" = auto
	ResponseSchema  map[string]any // JSON Schema; when set, Text is JSON
	Thinking        ThinkingLevel  // "" = default level from config
	MaxOutputTokens int            // 0 = model default
}

type LLMUsage struct {
	InputTokens      int
	AudioInputTokens int
	OutputTokens     int
	ThoughtTokens    int
	TotalTokens      int
}

type LLMResponse struct {
	ID            string // pass as PreviousID to continue the conversation
	Model         string
	Text          string
	FunctionCalls []LLMFunctionCall // run them, then send the results in the next request
	Usage         LLMUsage
}

// LanguageModel runs one model step per Generate call; the caller runs any
// function calls. The provider keeps the conversation, so an old ID can
// expire. System, Tools and ResponseSchema must be sent on every call.
type LanguageModel interface {
	Generate(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}
