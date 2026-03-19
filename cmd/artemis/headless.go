package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/agent/roles"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/lsp"
	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
)

// runChat handles one-shot chat: artemis chat [--multi] [--agent NAME] "message"
func runChat(args []string) {
	multi := false
	race := false
	agentRole := "coder"
	workDir := ""
	var messageParts []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--multi":
			multi = true
		case "--race":
			race = true
		case "--agent":
			if i+1 < len(args) {
				i++
				agentRole = args[i]
			}
		case "--dir":
			if i+1 < len(args) {
				i++
				workDir = args[i]
			}
		default:
			messageParts = append(messageParts, args[i])
		}
	}

	message := strings.Join(messageParts, " ")
	if message == "" {
		// Read from stdin if no message provided
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		message = strings.Join(lines, "\n")
	}

	if message == "" {
		fmt.Fprintln(os.Stderr, "Error: no message provided")
		os.Exit(1)
	}

	rt := newHeadlessRuntime(workDir)
	defer rt.shutdown()

	if race {
		rt.runRace(message, agentRole)
	} else if multi {
		rt.runOrchestrated(message)
	} else {
		rt.runSingle(message, agentRole)
	}
}

// runHeadless starts an interactive headless session (stdin/stdout loop).
func runHeadless(args []string) {
	workDir := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			i++
			workDir = args[i]
		}
	}

	rt := newHeadlessRuntime(workDir)
	defer rt.shutdown()

	fmt.Println("Artemis Headless Mode (type 'exit' to quit, '/multi' to toggle multi-agent)")
	fmt.Printf("Provider: %s | Agent: %s | Multi: %v\n", rt.providerName, "coder", rt.multi)
	fmt.Println("---")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large inputs

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		if input == "/multi" {
			rt.multi = !rt.multi
			fmt.Printf("[Multi-agent: %v]\n", rt.multi)
			continue
		}
		if input == "/help" {
			fmt.Println("Commands: /multi (toggle), /clear (history), /help, exit")
			continue
		}
		if input == "/clear" {
			rt.history = nil
			fmt.Println("[History cleared]")
			continue
		}

		if rt.multi {
			rt.runOrchestrated(input)
		} else {
			rt.runSingle(input, "coder")
		}
	}
}

// --- Headless Runtime ---

type headlessRuntime struct {
	cfg          config.Config
	provider     llm.Provider
	providerName string
	toolExec     *tools.ToolExecutor
	lspMgr       *lsp.Manager
	history      []llm.Message
	multi        bool
	projectRules string
}

func newHeadlessRuntime(workDir string) *headlessRuntime {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	if workDir == "" {
		workDir, _ = os.Getwd()
		if workDir == "" {
			workDir = "."
		}
	}

	// Find an available provider
	providerName := ""
	var provider llm.Provider
	for _, name := range []string{"gemini", "claude", "gpt", "glm"} {
		p := llm.NewProvider(name, &cfg)
		if p != nil {
			provider = p
			providerName = name
			break
		}
	}

	if provider == nil {
		fmt.Fprintln(os.Stderr, "Error: no LLM provider configured. Run 'artemis' (TUI) to set up API keys.")
		os.Exit(1)
	}

	// Tool executor
	te := tools.NewToolExecutor(workDir)

	// LSP (optional, best-effort)
	var lspMgr *lsp.Manager
	if cfg.LSP.Enabled {
		serverConfigs := make(map[string]lsp.ServerConfig)
		for lang, sc := range cfg.LSP.Servers {
			serverConfigs[lang] = lsp.ServerConfig{
				Command: sc.Command,
				Args:    sc.Args,
				Enabled: sc.Enabled,
			}
		}
		lspMgr = lsp.NewManager(workDir, serverConfigs)
		te.SetLSPManager(lspMgr)
	}

	// Load project rules
	projectRules := agent.LoadProjectRules(workDir)

	return &headlessRuntime{
		cfg:          cfg,
		provider:     provider,
		providerName: providerName,
		toolExec:     te,
		lspMgr:       lspMgr,
		projectRules: projectRules,
	}
}

func (rt *headlessRuntime) shutdown() {
	if rt.lspMgr != nil {
		rt.lspMgr.Shutdown()
	}
}

// runSingle executes a single agent with tools.
func (rt *headlessRuntime) runSingle(message, agentRole string) {
	eb := bus.NewEventBus(64)

	// Print events in real-time
	go func() {
		for evt := range eb.Chan() {
			switch evt.Type {
			case bus.EventAgentProgress:
				fmt.Fprintf(os.Stderr, "  [%s] %s\n", evt.AgentName, evt.Message)
			case bus.EventFileChanged:
				fmt.Fprintf(os.Stderr, "  ~ %s\n", evt.Message)
			case bus.EventAgentFail:
				fmt.Fprintf(os.Stderr, "  ✗ %s: %s\n", evt.AgentName, evt.Message)
			}
		}
	}()

	ag := roles.NewRoleAgent(agent.Role(agentRole), rt.provider, eb, rt.toolExec)
	ag.SetTask(message)
	if rt.projectRules != "" {
		ag.SetProjectRules(rt.projectRules)
		// TODO: CodeIndex requires VectorStore — init when Voyage API key available
	}

	ss := state.NewSessionState()
	ss.SetPhase("headless")

	ctx := context.Background() // no timeout

	err := ag.Run(ctx, ss)
	eb.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		return
	}

	// Get output from artifacts
	output := lastArtifactContent(ss)
	if output != "" {
		fmt.Println(output)
	}

	// Update conversation history
	rt.history = append(rt.history, llm.Message{Role: "user", Content: message})
	if output != "" {
		rt.history = append(rt.history, llm.Message{Role: "assistant", Content: output})
	}
}

// runOrchestrated executes via the full Orchestrator → Engine pipeline.
func (rt *headlessRuntime) runOrchestrated(message string) {
	fmt.Fprintf(os.Stderr, "[Orchestrator] Analyzing intent...\n")

	// Step 1: Call Orchestrator
	orchPrompt := roles.BuildOrchestratorPrompt(nil)
	messages := []llm.Message{
		{Role: "system", Content: orchPrompt},
	}
	messages = append(messages, rt.history...)
	messages = append(messages, llm.Message{Role: "user", Content: message})

	ctx := context.Background() // no timeout

	resp, err := rt.provider.Send(ctx, messages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator failed: %v\n", err)
		// Fallback to single agent
		fmt.Fprintf(os.Stderr, "Falling back to single agent...\n")
		rt.runSingle(message, "coder")
		return
	}

	// Step 2: Parse plan
	orchResp, err := orchestrator.ParseOrchestratorResponse(resp)
	if err != nil {
		plan, err2 := orchestrator.ParsePlan(resp)
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "Plan parse failed, falling back to single agent\n")
			rt.runSingle(message, "coder")
			return
		}
		rt.executePlan(ctx, plan, message)
		return
	}

	fmt.Fprintf(os.Stderr, "[Orchestrator] Intent: %s\n", orchResp.Intent)

	if orchResp.Intent == "trivial" || orchResp.Intent == "conversational" {
		agentRole := orchResp.DirectAgent
		if agentRole == "" {
			agentRole = "coder"
		}
		task := orchResp.DirectTask
		if task == "" {
			task = message
		}
		rt.runSingle(task, agentRole)
		return
	}

	plan := orchResp.ToExecutionPlan()
	if plan == nil {
		rt.runSingle(message, "coder")
		return
	}

	rt.executePlan(ctx, plan, message)
}

func (rt *headlessRuntime) executePlan(ctx context.Context, plan *orchestrator.ExecutionPlan, message string) {
	fmt.Fprintf(os.Stderr, "[Engine] Executing %d steps\n", len(plan.Steps))
	for i, step := range plan.Steps {
		for _, t := range step.Tasks {
			fmt.Fprintf(os.Stderr, "  Step %d: %s — %.60s\n", i+1, t.Agent, t.Task)
		}
	}

	eb := bus.NewEventBus(64)

	// Print events
	go func() {
		for evt := range eb.Chan() {
			switch evt.Type {
			case bus.EventAgentStart:
				fmt.Fprintf(os.Stderr, "  >> %s\n", evt.AgentName)
			case bus.EventAgentProgress:
				fmt.Fprintf(os.Stderr, "  .. %s: %s\n", evt.AgentName, evt.Message)
			case bus.EventAgentComplete:
				fmt.Fprintf(os.Stderr, "  << %s done\n", evt.AgentName)
			case bus.EventAgentFail:
				fmt.Fprintf(os.Stderr, "  ✗ %s: %s\n", evt.AgentName, evt.Message)
			case bus.EventFileChanged:
				fmt.Fprintf(os.Stderr, "  ~ %s\n", evt.Message)
			}
		}
	}()

	ss := state.NewSessionState()
	ss.SetPhase("headless")

	buildAgent := func(task orchestrator.AgentTask) agent.Agent {
		ag := roles.NewRoleAgent(agent.Role(task.Agent), rt.provider, eb, rt.toolExec)
		ag.SetTask(task.Task)
		ag.SetCritical(task.Critical)
		if rt.projectRules != "" {
			ag.SetProjectRules(rt.projectRules)
			// TODO: CodeIndex requires VectorStore — init when Voyage API key available
		}
		return ag
	}

	engine := orchestrator.NewEngine(nil, eb, nil, nil)
	result := engine.RunPlan(ctx, plan, ss, buildAgent)
	eb.Close()

	if result.Completed {
		fmt.Fprintf(os.Stderr, "[Engine] Completed (%d phases)\n", len(result.PhaseResults))
	} else {
		fmt.Fprintf(os.Stderr, "[Engine] Halted at %s: %v\n", result.HaltedAt, result.HaltError)
	}

	// Output the last agent's result
	output := lastArtifactContent(ss)
	if output != "" {
		fmt.Println(output)
	}

	rt.history = append(rt.history, llm.Message{Role: "user", Content: message})
	if output != "" {
		rt.history = append(rt.history, llm.Message{Role: "assistant", Content: output})
	}
}

// runRace executes the same task with 2 different providers in parallel,
// using isolated git worktrees. Picks the result that compiles/passes verification.
func (rt *headlessRuntime) runRace(message, agentRole string) {
	fmt.Fprintf(os.Stderr, "[Race] Starting competitive execution with 2 providers...\n")

	// Find 2 available providers
	providers := rt.findProviders(2)
	if len(providers) < 2 {
		fmt.Fprintf(os.Stderr, "[Race] Only %d provider(s) available, falling back to single\n", len(providers))
		rt.runSingle(message, agentRole)
		return
	}

	// Check if we're in a git repo
	cwd, _ := os.Getwd()
	wtMgr := tools.NewParallelWorktreeManager(cwd)
	defer wtMgr.CleanupAll()

	type raceResult struct {
		idx      int
		provider string
		output   string
		err      error
		diff     string
	}

	results := make(chan raceResult, 2)

	for i, prov := range providers {
		go func(idx int, p llm.Provider, pName string) {
			// Create isolated worktree
			wt, te, err := wtMgr.Create(context.Background(), fmt.Sprintf("race-%d", idx))
			if err != nil {
				results <- raceResult{idx: idx, provider: pName, err: err}
				return
			}
			defer wt.Clean()

			eb := bus.NewEventBus(32)
			go func() {
				for range eb.Chan() {
				}
			}()

			ag := roles.NewRoleAgent(agent.Role(agentRole), p, eb, te)
			ag.SetTask(message)
			if rt.projectRules != "" {
				ag.SetProjectRules(rt.projectRules)
			}

			ss := state.NewSessionState()
			ss.SetPhase("race")

			raceCtx := context.Background() // no timeout

			err = ag.Run(raceCtx, ss)
			eb.Close()

			diff, _ := wtMgr.GetDiff(context.Background(), wt)

			output := lastArtifactContent(ss)

			results <- raceResult{
				idx:      idx,
				provider: pName,
				output:   output,
				err:      err,
				diff:     diff,
			}
		}(i, prov.provider, prov.name)
	}

	// Collect results
	var collected []raceResult
	for i := 0; i < len(providers); i++ {
		r := <-results
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "[Race] Provider %s failed: %v\n", r.provider, r.err)
		} else {
			fmt.Fprintf(os.Stderr, "[Race] Provider %s completed (%d chars output, %d chars diff)\n",
				r.provider, len(r.output), len(r.diff))
		}
		collected = append(collected, r)
	}

	// Pick best result — prefer one with diff (made changes) and no error
	var best *raceResult
	for i := range collected {
		r := &collected[i]
		if r.err != nil {
			continue
		}
		if best == nil || (len(r.diff) > len(best.diff)) {
			best = r
		}
	}

	if best == nil {
		fmt.Fprintf(os.Stderr, "[Race] All providers failed\n")
		return
	}

	fmt.Fprintf(os.Stderr, "[Race] Winner: %s\n", best.provider)
	if best.output != "" {
		fmt.Println(best.output)
	}

	// Apply winning diff to main repo
	if best.diff != "" {
		fmt.Fprintf(os.Stderr, "[Race] Applying winning changes to main repo...\n")
		// Re-create a temporary worktree ref to apply
		// Actually, just apply the diff directly
		cmd := exec.CommandContext(context.Background(), "git", "apply", "--allow-empty")
		cmd.Dir = cwd
		cmd.Stdin = strings.NewReader(best.diff)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "[Race] Warning: failed to apply diff: %v\n%s\n", err, string(out))
		} else {
			fmt.Fprintf(os.Stderr, "[Race] Changes applied successfully\n")
		}
	}
}

type providerEntry struct {
	name     string
	provider llm.Provider
}

func (rt *headlessRuntime) findProviders(n int) []providerEntry {
	var result []providerEntry
	for _, name := range []string{"gemini", "claude", "gpt", "glm"} {
		p := llm.NewProvider(name, &rt.cfg)
		if p != nil {
			result = append(result, providerEntry{name: name, provider: p})
			if len(result) >= n {
				break
			}
		}
	}
	return result
}

// lastArtifactContent returns the content of the last artifact in the session state.
func lastArtifactContent(ss *state.SessionState) string {
	artifacts := ss.GetArtifacts()
	if len(artifacts) == 0 {
		return ""
	}
	return artifacts[len(artifacts)-1].Content
}
