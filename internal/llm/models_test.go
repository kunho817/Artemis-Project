package llm

import "testing"

func TestGetModelSpec_KnownModels(t *testing.T) {
	tests := []struct {
		model         string
		wantContext   int
		wantMaxOutput int
	}{
		{"claude-sonnet-4-6", 200_000, 16_000},
		{"gemini-3.1-pro-preview", 2_000_000, 65_000},
		{"gpt-5.4", 400_000, 32_000},
		{"glm-5", 200_000, 8_000},
		{"qwen2.5-coder-7b", 32_768, 8_192},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			spec := GetModelSpec(tt.model)
			if spec.ContextWindow != tt.wantContext {
				t.Errorf("ContextWindow = %d, want %d", spec.ContextWindow, tt.wantContext)
			}
			if spec.MaxOutput != tt.wantMaxOutput {
				t.Errorf("MaxOutput = %d, want %d", spec.MaxOutput, tt.wantMaxOutput)
			}
		})
	}
}

func TestGetModelSpec_UnknownModel(t *testing.T) {
	spec := GetModelSpec("nonexistent-model-xyz")
	if spec.ContextWindow != 32_768 {
		t.Errorf("fallback ContextWindow = %d, want 32768", spec.ContextWindow)
	}
	if spec.MaxOutput != 4_096 {
		t.Errorf("fallback MaxOutput = %d, want 4096", spec.MaxOutput)
	}
}

func TestAvailableInputTokens(t *testing.T) {
	spec := ModelSpec{ContextWindow: 200_000, MaxOutput: 16_000}
	got := spec.AvailableInputTokens()
	want := 184_000
	if got != want {
		t.Errorf("AvailableInputTokens() = %d, want %d", got, want)
	}
}

func TestRegisterModelSpec(t *testing.T) {
	model := "test-custom-model-42"
	spec := ModelSpec{ContextWindow: 50_000, MaxOutput: 10_000}

	RegisterModelSpec(model, spec)
	defer func() {
		// cleanup: remove from registry
		modelRegistry.mu.Lock()
		delete(modelRegistry.specs, model)
		modelRegistry.mu.Unlock()
	}()

	got := GetModelSpec(model)
	if got.ContextWindow != 50_000 {
		t.Errorf("registered ContextWindow = %d, want 50000", got.ContextWindow)
	}
	if !HasModelSpec(model) {
		t.Error("HasModelSpec returned false for registered model")
	}
}

func TestHasModelSpec(t *testing.T) {
	if !HasModelSpec("gpt-5.4") {
		t.Error("expected true for known model gpt-5.4")
	}
	if HasModelSpec("totally-fake-model") {
		t.Error("expected false for unknown model")
	}
}

func TestAllModelSpecs_ReturnsCopy(t *testing.T) {
	all := AllModelSpecs()
	if len(all) == 0 {
		t.Fatal("AllModelSpecs returned empty map")
	}
	// Mutating the copy should not affect the registry
	all["mutated"] = ModelSpec{ContextWindow: 1, MaxOutput: 1}
	if HasModelSpec("mutated") {
		t.Error("mutation of AllModelSpecs copy affected registry")
	}
}
