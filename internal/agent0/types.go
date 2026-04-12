package agent0

import "time"

// Thread represents an agent0 conversation thread.
type Thread struct {
	ID             string     `json:"id"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	Name           string     `json:"name,omitempty"`
	OriginURL      string     `json:"originUrl,omitempty"`
	ParentThreadID string     `json:"parentThreadId,omitempty"`
	NetworkLevel   string     `json:"networkLevel,omitempty"`
}

// Message is a conversation message. The Role field determines which fields are populated.
type Message struct {
	Role    string `json:"role"`
	ID      string `json:"id"`
	Hash    string `json:"hash"`
	Content string `json:"content"`

	// Common optional fields
	UserID    string     `json:"userId,omitempty"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`

	// Error-specific fields
	StatusCode string `json:"statusCode,omitempty"`

	// Metadata-specific fields
	Suggestions []string `json:"suggestions,omitempty"`

	// Sub-agent thread fields
	MainAgent string    `json:"mainAgent,omitempty"`
	SubAgent  string    `json:"subAgent,omitempty"`
	Why       string    `json:"why,omitempty"`
	Messages  []Message `json:"messages,omitempty"`

	// Agent selection fields
	Agent string `json:"agent,omitempty"`

	Summarized bool `json:"summarized,omitempty"`
}

// InvokeRequest is the request body for POST /agent0-sdk/invoke.
type InvokeRequest struct {
	Message        string `json:"message"`
	Dataset        string `json:"dataset"`
	ThreadID       string `json:"threadId,omitempty"`
	NetworkLevel   string `json:"networkLevel,omitempty"`
	TimezoneOffset *int   `json:"timezoneOffset,omitempty"`
}

// InvokeResponse is the SSE data payload from the invoke endpoint.
type InvokeResponse struct {
	Thread   Thread    `json:"thread"`
	Messages []Message `json:"messages"`
}

// ThreadResponse is the response from POST /agent0-sdk/thread.
type ThreadResponse struct {
	Thread   Thread    `json:"thread"`
	Messages []Message `json:"messages"`
}

// Message role constants.
const (
	RoleHuman          = "human"
	RoleAssistant      = "assistant"
	RoleThinking       = "thinking"
	RoleTool           = "tool"
	RoleMetadata       = "metadata"
	RoleError          = "error"
	RoleCancel         = "cancel"
	RoleSummary        = "summary"
	RoleAgentSelection = "agent_selection"
	RoleSubAgentThread = "sub_agent_thread"
)

// IsDisplayable returns true if the message should be shown to users by default (not verbose mode).
func (m Message) IsDisplayable() bool {
	switch m.Role {
	case RoleHuman, RoleAssistant, RoleError, RoleCancel:
		return true
	case RoleMetadata:
		return len(m.Suggestions) > 0
	default:
		return false
	}
}

// IsVerbose returns true if the message should only be shown in verbose mode.
func (m Message) IsVerbose() bool {
	switch m.Role {
	case RoleThinking, RoleTool, RoleAgentSelection, RoleSubAgentThread:
		return true
	default:
		return false
	}
}
