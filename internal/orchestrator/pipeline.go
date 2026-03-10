package orchestrator

import (
	"fmt"

	"github.com/artemis-project/artemis/internal/agent"
)

// PhaseName identifies a pipeline phase.
type PhaseName string

const (
	PhaseAnalysis       PhaseName = "analysis"
	PhasePlanning       PhaseName = "planning"
	PhaseArchitecture   PhaseName = "architecture"
	PhaseImplementation PhaseName = "implementation"
	PhaseVerification   PhaseName = "verification"
)

// Phase represents a single step in the pipeline.
// Agents within a phase run in parallel via errgroup.
type Phase struct {
	Name   PhaseName
	Agents []agent.Agent
}

// NewPhase creates a phase with the given agents.
func NewPhase(name PhaseName, agents ...agent.Agent) Phase {
	return Phase{
		Name:   name,
		Agents: agents,
	}
}

// CriticalAgents returns agents whose failure should halt the pipeline.
func (p *Phase) CriticalAgents() []agent.Agent {
	var critical []agent.Agent
	for _, a := range p.Agents {
		if a.Critical() {
			critical = append(critical, a)
		}
	}
	return critical
}

// Pipeline defines the full sequence of phases.
type Pipeline struct {
	Phases []Phase
}

// DefaultPipeline returns the standard 5-phase pipeline.
// Agents must be injected after creation via SetPhaseAgents.
func DefaultPipeline() *Pipeline {
	return &Pipeline{
		Phases: []Phase{
			{Name: PhaseAnalysis},
			{Name: PhasePlanning},
			{Name: PhaseArchitecture},
			{Name: PhaseImplementation},
			{Name: PhaseVerification},
		},
	}
}

// SetPhaseAgents assigns agents to a named phase.
func (p *Pipeline) SetPhaseAgents(name PhaseName, agents ...agent.Agent) error {
	for i := range p.Phases {
		if p.Phases[i].Name == name {
			p.Phases[i].Agents = agents
			return nil
		}
	}
	return fmt.Errorf("phase %q not found in pipeline", name)
}

// GetPhase returns a pointer to the named phase, or nil.
func (p *Pipeline) GetPhase(name PhaseName) *Phase {
	for i := range p.Phases {
		if p.Phases[i].Name == name {
			return &p.Phases[i]
		}
	}
	return nil
}

// Validate checks that every phase has at least one agent.
func (p *Pipeline) Validate() error {
	for _, phase := range p.Phases {
		if len(phase.Agents) == 0 {
			return fmt.Errorf("phase %q has no agents assigned", phase.Name)
		}
	}
	return nil
}
