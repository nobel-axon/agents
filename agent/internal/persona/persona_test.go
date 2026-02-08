package persona

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPersona(t *testing.T) {
	t.Run("returns correct personas", func(t *testing.T) {
		p1 := GetPersona("agent-1")
		assert.Equal(t, "The Analyst", p1.Name)
		assert.Contains(t, p1.Traits, "technical")

		p2 := GetPersona("agent-2")
		assert.Equal(t, "The Hype Master", p2.Name)
		assert.Contains(t, p2.Traits, "energetic")

		p3 := GetPersona("agent-3")
		assert.Equal(t, "The Wit", p3.Name)
		assert.Contains(t, p3.Traits, "sardonic")
	})

	t.Run("defaults to agent-1 for unknown", func(t *testing.T) {
		p := GetPersona("unknown-agent")
		assert.Equal(t, "The Analyst", p.Name)
	})
}

func TestPersona_GetTemplate(t *testing.T) {
	p := GetPersona("agent-2") // Hype Master

	t.Run("question_live template", func(t *testing.T) {
		text := p.GetTemplate("question_live", "crypto", 3, "number")
		assert.NotEmpty(t, text)
		assert.Contains(t, text, "crypto")
	})

	t.Run("answer_correct template", func(t *testing.T) {
		text := p.GetTemplate("answer_correct", "0xabc", 1.5)
		assert.NotEmpty(t, text)
		assert.Contains(t, text, "0xabc")
	})

	t.Run("unknown event returns default", func(t *testing.T) {
		text := p.GetTemplate("unknown_event")
		assert.Contains(t, text, "unknown_event")
	})
}

func TestPersonas_HaveDistinctStyles(t *testing.T) {
	// Ensure each persona has unique system prompts
	p1 := GetPersona("agent-1")
	p2 := GetPersona("agent-2")
	p3 := GetPersona("agent-3")

	assert.NotEqual(t, p1.SystemPrompt, p2.SystemPrompt)
	assert.NotEqual(t, p2.SystemPrompt, p3.SystemPrompt)
	assert.NotEqual(t, p1.SystemPrompt, p3.SystemPrompt)

	// Each should have templates for key events
	events := []string{"question_live", "answer_correct", "match_settled"}
	for _, event := range events {
		assert.NotEmpty(t, p1.Templates[event], "agent-1 missing %s", event)
		assert.NotEmpty(t, p2.Templates[event], "agent-2 missing %s", event)
		assert.NotEmpty(t, p3.Templates[event], "agent-3 missing %s", event)
	}
}
