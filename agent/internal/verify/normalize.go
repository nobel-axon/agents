// Package verify provides answer verification and normalization.
package verify

import (
	"regexp"
	"strings"
)

// FormatHint defines the expected answer format.
type FormatHint string

const (
	FormatNumber  FormatHint = "number"
	FormatHex     FormatHint = "hex"
	FormatAddress FormatHint = "address"
	FormatName    FormatHint = "name"
	FormatText    FormatHint = "text"
)

var (
	multiSpaceRegex   = regexp.MustCompile(`\s+`)
	punctuationRegex  = regexp.MustCompile(`^[^\w]+|[^\w]+$`)
	titleRegex        = regexp.MustCompile(`(?i)^(dr|mr|mrs|ms|miss|prof|sir|lady)\.?\s+`)
	articleRegex      = regexp.MustCompile(`(?i)^(the|a|an)\s+`)
	nonDigitRegex     = regexp.MustCompile(`[^\d.\-]`)
)

// Normalize applies normalization rules to an answer based on format hint.
func Normalize(answer string, format FormatHint) string {
	// General normalization (always applied)
	result := strings.TrimSpace(answer)
	result = strings.ToLower(result)

	// Format-specific normalization (some formats handle punctuation differently)
	switch format {
	case FormatNumber:
		// Numbers need special handling to preserve minus sign
		return normalizeNumber(result)
	case FormatHex:
		result = punctuationRegex.ReplaceAllString(result, "")
		return normalizeHex(result)
	case FormatAddress:
		result = punctuationRegex.ReplaceAllString(result, "")
		return normalizeAddress(result)
	case FormatName:
		result = punctuationRegex.ReplaceAllString(result, "")
		result = multiSpaceRegex.ReplaceAllString(result, " ")
		return normalizeName(result)
	case FormatText:
		result = punctuationRegex.ReplaceAllString(result, "")
		result = multiSpaceRegex.ReplaceAllString(result, " ")
		return normalizeText(result)
	default:
		result = punctuationRegex.ReplaceAllString(result, "")
		result = multiSpaceRegex.ReplaceAllString(result, " ")
		return strings.TrimSpace(result)
	}
}

// normalizeNumber removes formatting from numbers.
func normalizeNumber(s string) string {
	// Remove commas and spaces
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	// Keep only digits, decimal point, and minus sign
	s = nonDigitRegex.ReplaceAllString(s, "")
	return s
}

// normalizeHex normalizes hex values.
func normalizeHex(s string) string {
	s = strings.ToLower(s)
	// Ensure 0x prefix
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + s
	}
	return s
}

// normalizeAddress normalizes Ethereum addresses.
func normalizeAddress(s string) string {
	s = strings.ToLower(s)
	// Ensure 0x prefix
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + s
	}
	// Validate length (0x + 40 hex chars)
	if len(s) != 42 {
		return s // Return as-is if invalid length
	}
	return s
}

// normalizeName normalizes names by removing titles.
func normalizeName(s string) string {
	s = strings.ToLower(s)
	s = titleRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// normalizeText normalizes text by removing leading articles.
func normalizeText(s string) string {
	s = strings.ToLower(s)
	s = articleRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
