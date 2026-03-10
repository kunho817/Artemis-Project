package tools

import (
	"testing"
)

func TestIsValidGitRef(t *testing.T) {
	valid := []string{"HEAD", "main", "HEAD~1", "origin/main", "v1.0.0", "feature/my-branch"}
	for _, ref := range valid {
		if !isValidGitRef(ref) {
			t.Errorf("expected %q to be valid", ref)
		}
	}

	invalid := []string{"", "ref with space", "ref;rm -rf", "ref$(cmd)", "ref`cmd`", "ref|pipe"}
	for _, ref := range invalid {
		if isValidGitRef(ref) {
			t.Errorf("expected %q to be invalid", ref)
		}
	}
}
