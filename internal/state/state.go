package state

import (
	"sync"
	"time"
)

// ArtifactType identifies the kind of data an artifact holds.
type ArtifactType string

const (
	ArtifactUserRequest      ArtifactType = "USER_REQUEST"
	ArtifactAnalysis         ArtifactType = "ANALYSIS"
	ArtifactSearchResult     ArtifactType = "SEARCH_RESULT"
	ArtifactExploration      ArtifactType = "EXPLORATION"
	ArtifactPlan             ArtifactType = "PLAN"
	ArtifactArchitecture     ArtifactType = "ARCHITECTURE"
	ArtifactCode             ArtifactType = "CODE_CHANGE"
	ArtifactDesign           ArtifactType = "DESIGN"
	ArtifactTestResult       ArtifactType = "TEST_RESULT"
	ArtifactQAReport         ArtifactType = "QA_REPORT"
	ArtifactOrchestratorPlan ArtifactType = "ORCHESTRATOR_PLAN"
	ArtifactError            ArtifactType = "ERROR"
	ArtifactConsultation     ArtifactType = "CONSULTATION"
	ArtifactReview           ArtifactType = "REVIEW"       // Phase C-6: review feedback from feedback loop
)

// Artifact represents a piece of data produced by an agent.
type Artifact struct {
	Type      ArtifactType
	Source    string // Agent name that produced this
	Content   string
	Metadata  map[string]string
	CreatedAt time.Time
}

// SessionState is the thread-safe blackboard shared by all agents.
type SessionState struct {
	mu        sync.RWMutex
	id        string     // pipeline run ID ("run_<nano>")
	sessionID string     // owning TUI session ID
	parentID  string     // parent run ID (for background tasks)
	artifacts []Artifact
	phase     string   // current pipeline phase
	history   []string // conversation history from prior turns ("role: content")
}

// NewSessionState creates a new empty session state.
func NewSessionState() *SessionState {
	return &SessionState{
		artifacts: make([]Artifact, 0),
	}
}

// NewSessionStateWithID creates a session state with explicit IDs for hierarchy tracking.
func NewSessionStateWithID(id, sessionID, parentID string) *SessionState {
	return &SessionState{
		id:        id,
		sessionID: sessionID,
		parentID:  parentID,
		artifacts: make([]Artifact, 0),
	}
}

// AddArtifact appends an artifact to the session.
func (s *SessionState) AddArtifact(a Artifact) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]string)
	}
	s.artifacts = append(s.artifacts, a)
}

// GetArtifacts returns all artifacts.
func (s *SessionState) GetArtifacts() []Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Artifact, len(s.artifacts))
	copy(out, s.artifacts)
	return out
}

// GetByType returns artifacts filtered by type.
func (s *SessionState) GetByType(t ArtifactType) []Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Artifact
	for _, a := range s.artifacts {
		if a.Type == t {
			out = append(out, a)
		}
	}
	return out
}

// GetBySource returns artifacts from a specific agent.
func (s *SessionState) GetBySource(source string) []Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Artifact
	for _, a := range s.artifacts {
		if a.Source == source {
			out = append(out, a)
		}
	}
	return out
}

// SetPhase sets the current pipeline phase name.
func (s *SessionState) SetPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
}

// Phase returns the current pipeline phase.
func (s *SessionState) Phase() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phase
}

// SetHistory stores conversation history for agent context.
func (s *SessionState) SetHistory(history []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = history
}

// HistorySummary returns a formatted summary of conversation history.
// Returns empty string if no history exists.
func (s *SessionState) HistorySummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.history) == 0 {
		return ""
	}
	var result string
	for _, h := range s.history {
		result += h + "\n"
	}
	return result
}

// Summary builds a text summary of all artifacts for injection into agent prompts.
func (s *SessionState) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.artifacts) == 0 {
		return ""
	}

	var result string
	for _, a := range s.artifacts {
		result += "[" + string(a.Type) + " from " + a.Source + "]\n"
		result += a.Content + "\n\n"
	}
	return result
}

// Clear resets the session state.
func (s *SessionState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = make([]Artifact, 0)
	s.phase = ""
}

// ID returns the pipeline run ID.
func (s *SessionState) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

// SetID sets the pipeline run ID.
func (s *SessionState) SetID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
}

// SessionID returns the owning TUI session ID.
func (s *SessionState) SessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionID
}

// SetSessionID sets the owning TUI session ID.
func (s *SessionState) SetSessionID(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionID = sessionID
}

// ParentID returns the parent pipeline run ID (for background tasks).
func (s *SessionState) ParentID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.parentID
}

// SetParentID sets the parent pipeline run ID.
func (s *SessionState) SetParentID(parentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parentID = parentID
}
