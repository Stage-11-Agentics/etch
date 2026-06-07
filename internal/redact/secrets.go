package redact

import (
	"fmt"
	"regexp"
)

type secretPattern struct {
	Name  string
	Regex *regexp.Regexp
	// Validate, when non-nil, decides whether a regex match is really a
	// secret. RE2 has no lookarounds, so class-composition checks (e.g.
	// "must contain upper+lower+digit") live in code instead.
	Validate func(match string) bool
}

// Pattern order is semantic — do not reorder casually:
//   - the full private-key block runs before the header-only fallback
//   - bearer-token runs before jwt so "Bearer <jwt>" keeps its more
//     specific marker name
//   - labeled secrets run before the bare 40-char AWS form
var builtinPatterns = []secretPattern{
	{
		// Full PEM block: header, key material, and END line (ETCH-28).
		// [A-Z ]* covers RSA/EC/DSA plus OPENSSH/ENCRYPTED variants.
		Name:  "private-key",
		Regex: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	},
	{
		// Truncated-block fallback (no END marker): the header plus any
		// following long base64 lines. Material lines are 40+ chars, so
		// ordinary prose after a lone header is left alone.
		Name:  "private-key",
		Regex: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----(?:\s*[A-Za-z0-9+/=]{40,})*`),
	},
	{
		Name:  "bearer-token",
		Regex: regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._\-]+`),
	},
	{
		// Bare JWT: three base64url segments; eyJ is base64 of `{"`
		// (ETCH-27). Minimum segment lengths keep x.y.z prose safe.
		Name:  "jwt",
		Regex: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{6,}`),
	},
	{
		// Real Anthropic keys carry a tier segment (api03-, oat01-,
		// sid01-) and a long body; doc placeholders like sk-ant-EXAMPLE
		// must pass through (ETCH-29).
		Name:  "anthropic-api-key",
		Regex: regexp.MustCompile(`sk-ant-[a-z]{2,8}[0-9]{2}-[A-Za-z0-9_-]{16,}`),
	},
	{
		// Modern prefixed OpenAI keys (ETCH-40 finding 7). Deliberately
		// not a generic sk-[\w-]{20,}: that would clobber doc strings
		// like sk-DOCUMENTATION-NOT-A-KEY.
		Name:  "openai-api-key",
		Regex: regexp.MustCompile(`sk-(?:proj|svcacct|admin)-[A-Za-z0-9_-]{20,}`),
	},
	{
		// Legacy unprefixed OpenAI keys.
		Name:  "openai-api-key",
		Regex: regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
	},
	{
		Name:  "stripe-live-key",
		Regex: regexp.MustCompile(`sk_live_[a-zA-Z0-9]+`),
	},
	{
		Name:  "stripe-test-key",
		Regex: regexp.MustCompile(`sk_test_[a-zA-Z0-9]+`),
	},
	{
		Name:  "aws-access-key",
		Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:  "aws-secret-key",
		Regex: regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*\S+`),
	},
	{
		// Generic credential assignments (ETCH-39 + ETCH-26's AWS_SECRET=
		// variants). The keyword must be immediately followed by : or =,
		// so prose like "tokens: 4096" or "max_tokens=8192" is untouched.
		// The value class includes /+= for base64-shaped values.
		Name:  "generic-secret",
		Regex: regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|secret[_-]?key|client[_-]?secret|password|passwd|pwd|pass|token|secret)\s*[:=]\s*["']?[A-Za-z0-9_\-/+=]{8,}["']?`),
	},
	{
		// Bare AWS secret access key (ETCH-26): exactly 40 base64 chars
		// with mixed character classes. {40,} grabs the maximal run, so
		// the len==40 validator rejects 40-hex git SHAs (single-case /
		// all-hex), sha256 hex (64 chars), and longer base64 blobs.
		// Known, accepted miss: a real 40-char secret embedded INSIDE a
		// longer contiguous base64 run is not redacted (best-effort).
		// Runs last so labeled patterns win their specific marker names.
		Name:     "aws-secret-key",
		Regex:    regexp.MustCompile(`[A-Za-z0-9/+=]{40,}`),
		Validate: looksLikeBareAWSSecret,
	},
}

// looksLikeBareAWSSecret reports whether a maximal base64 run has the
// shape of a bare AWS secret access key: exactly 40 chars, mixed case,
// at least one digit, and not a plain hex string (which would be a SHA).
func looksLikeBareAWSSecret(m string) bool {
	if len(m) != 40 {
		return false
	}
	var hasUpper, hasLower, hasDigit, nonHex bool
	for _, c := range m {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
			if c > 'F' {
				nonHex = true
			}
		case c >= 'a' && c <= 'z':
			hasLower = true
			if c > 'f' {
				nonHex = true
			}
		case c >= '0' && c <= '9':
			hasDigit = true
		default: // '/', '+', '='
			nonHex = true
		}
	}
	return hasUpper && hasLower && hasDigit && nonHex
}

func ScanSecrets(text string) string {
	for _, p := range builtinPatterns {
		marker := fmt.Sprintf("[REDACTED:%s]", p.Name)
		text = p.Regex.ReplaceAllStringFunc(text, func(m string) string {
			if p.Validate != nil && !p.Validate(m) {
				return m
			}
			return marker
		})
	}
	return text
}

func compileCustomPatterns(patterns []string) []secretPattern {
	var result []secretPattern
	for i, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		result = append(result, secretPattern{
			Name:  fmt.Sprintf("custom-%d", i),
			Regex: re,
		})
	}
	return result
}
