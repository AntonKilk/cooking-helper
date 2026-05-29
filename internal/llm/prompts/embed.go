// Package prompts holds version-controlled LLM prompt templates, embedded into
// the binary. Prompts are named <task>.v<N>.txt so a new revision lands as a
// new file rather than an in-place edit.
package prompts

import (
	"embed"
	"fmt"
)

//go:embed *.txt
var fs embed.FS

// Load returns the contents of the named prompt file (e.g.
// "categorize_ingredient.v1.txt").
func Load(name string) (string, error) {
	b, err := fs.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("load prompt %q: %w", name, err)
	}
	return string(b), nil
}
