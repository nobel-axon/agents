package question

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerator_GenerateQuestion_WithLLM(t *testing.T) {
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			// Debate format skips self-validation, so only one response needed per call
			`{"question": "Is mathematics discovered or invented?", "answer": "Mathematics is discovered — it exists independently of human minds.", "variants": ["invented", "both"], "format": "debate"}`,
			`{"question": "What caused the Bronze Age Collapse?", "answer": "A systems collapse triggered by cascading failures.", "variants": ["Sea Peoples invasion", "climate change"], "format": "debate"}`,
			`{"question": "Does free will exist?", "answer": "Compatibilism offers the best framework.", "variants": ["hard determinism", "libertarian free will"], "format": "debate"}`,
			`{"question": "Is consciousness fundamental?", "answer": "Panpsychism suggests consciousness is a fundamental feature.", "variants": ["emergentism", "illusionism"], "format": "debate"}`,
		},
	}

	gen := NewGenerator(WithLLMClient(mock))

	t.Run("generates question via LLM", func(t *testing.T) {
		resp, err := gen.GenerateQuestion("match-1", "philosophy", 2, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Question)
		assert.NotEmpty(t, resp.AnswerHash)
		assert.True(t, len(resp.AnswerHash) == 66) // 0x + 64 hex chars
		assert.Equal(t, "debate", resp.FormatHint)
	})

	t.Run("stores answer internally", func(t *testing.T) {
		resp, err := gen.GenerateQuestion("match-3", "history", 3, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Question)

		answer, ok := gen.GetAnswer("match-3")
		assert.True(t, ok)
		assert.NotEmpty(t, answer.Answer)
	})

	t.Run("reveal deletes answer", func(t *testing.T) {
		_, err := gen.GenerateQuestion("match-4", "philosophy", 2, nil)
		require.NoError(t, err)

		reveal, err := gen.RevealAnswer("match-4")
		require.NoError(t, err)
		assert.NotEmpty(t, reveal.Answer)
		assert.True(t, len(reveal.Salt) > 2) // 0x prefix + hex

		// Should be deleted now
		_, ok := gen.GetAnswer("match-4")
		assert.False(t, ok)
	})

	t.Run("no LLM or buffer returns error immediately", func(t *testing.T) {
		genNoLLM := NewGenerator()
		_, err := genNoLLM.GenerateQuestion("match-5", "crypto", 1, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no question source available")
	})
}

func TestGenerator_GenerateQuestion_WithBuffer(t *testing.T) {
	// Create a buffer and manually insert a question
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := &mockLLMProvider{name: "test"}
	buf := NewQuestionBuffer(ctx, mock, []string{"crypto"}, 3)
	// Don't start the buffer — we'll insert manually

	buf.mu.Lock()
	buf.buffer["crypto"] = []BufferedQuestion{
		{
			Question:   "Is PoS fundamentally flawed?",
			Answer:     "PoS creates plutocratic governance structures.",
			Variants:   []string{"PoS is superior", "Neither is ideal"},
			Format:     "debate",
			Category:   "crypto",
			Difficulty: 2,
		},
	}
	buf.mu.Unlock()

	gen := NewGenerator(WithLLMClient(mock), WithPreStocking(buf))

	resp, err := gen.GenerateQuestion("match-buf", "crypto", 2, nil)
	require.NoError(t, err)
	assert.Equal(t, "Is PoS fundamentally flawed?", resp.Question)
	assert.Equal(t, "debate", resp.FormatHint)
	assert.NotEmpty(t, resp.AnswerHash)

	// Answer should be stored
	stored, ok := gen.GetAnswer("match-buf")
	assert.True(t, ok)
	assert.Equal(t, "PoS creates plutocratic governance structures.", stored.Answer)
}

func TestGenerator_GenerateQuestion_RetrySucceeds(t *testing.T) {
	// Mock LLM that fails the first call but succeeds on the second.
	// Debate format skips self-validation, so only one response per successful call.
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			"",
			`{"question": "Is DeFi sustainable?", "answer": "DeFi is sustainable with proper regulation.", "variants": ["no", "depends"], "format": "debate"}`,
		},
		errors: []error{
			errors.New("temporary LLM failure"),
			nil,
		},
	}

	gen := NewGenerator(WithLLMClient(mock))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := gen.GenerateQuestionWithContext(ctx, "match-retry", "crypto", 2, nil)
	require.NoError(t, err)
	assert.Equal(t, "Is DeFi sustainable?", resp.Question)
	assert.Equal(t, "debate", resp.FormatHint)
	assert.NotEmpty(t, resp.AnswerHash)
}

func TestGenerator_GenerateQuestion_ContextCancellation(t *testing.T) {
	// Mock LLM that always fails — verify context cancellation breaks the retry loop
	mock := &mockLLMProvider{
		name:      "test",
		responses: []string{"", "", "", ""},
		errors:    []error{errors.New("fail"), errors.New("fail"), errors.New("fail"), errors.New("fail")},
	}

	gen := NewGenerator(WithLLMClient(mock))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := gen.GenerateQuestionWithContext(ctx, "match-cancel", "science", 1, nil)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	// Should exit quickly after context cancellation, not wait the full 30s
	assert.Less(t, elapsed, 5*time.Second)
}

func TestAnswerHash_Deterministic(t *testing.T) {
	// Same answer + salt should produce same hash
	salt := []byte("test-salt-32-bytes-long-000000")
	hash1 := computeAnswerHash("21000000", salt)
	hash2 := computeAnswerHash("21000000", salt)
	assert.Equal(t, hash1, hash2)

	// Different answer should produce different hash
	hash3 := computeAnswerHash("21000001", salt)
	assert.NotEqual(t, hash1, hash3)
}
