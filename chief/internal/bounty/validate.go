package bounty

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateQuestion performs basic validation on a bounty question.
// This is a safety net to catch garbage/spam, not a content moderation system.
func ValidateQuestion(question string) error {
	trimmed := strings.TrimSpace(question)

	// Empty after trim
	if len(trimmed) == 0 {
		return fmt.Errorf("question is empty after trimming whitespace")
	}

	// Length check (raw bytes)
	if len(question) < 10 {
		return fmt.Errorf("question length %d below minimum 10", len(question))
	}
	if len(question) > 2000 {
		return fmt.Errorf("question length %d exceeds maximum 2000", len(question))
	}

	// Non-printable / control characters (allow newlines and tabs)
	for _, r := range trimmed {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return fmt.Errorf("question contains non-printable character: U+%04X", r)
		}
	}

	// Must contain at least one letter (not just numbers/symbols)
	hasLetter := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return fmt.Errorf("question contains no letters")
	}

	return nil
}
