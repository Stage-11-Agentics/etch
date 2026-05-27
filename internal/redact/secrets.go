package redact

import (
	"fmt"
	"regexp"
)

type secretPattern struct {
	Name    string
	Regex   *regexp.Regexp
}

var builtinPatterns = []secretPattern{
	{
		Name:  "aws-access-key",
		Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:  "aws-secret-key",
		Regex: regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*\S+`),
	},
	{
		Name:  "anthropic-api-key",
		Regex: regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]+`),
	},
	{
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
		Name:  "generic-secret",
		Regex: regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|access[_-]?token|secret[_-]?key)\s*[:=]\s*["']?[a-zA-Z0-9_\-]{16,}["']?`),
	},
	{
		Name:  "bearer-token",
		Regex: regexp.MustCompile(`Bearer\s+[a-zA-Z0-9._\-]+`),
	},
	{
		Name:  "private-key",
		Regex: regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
	},
}

func ScanSecrets(text string) string {
	for _, p := range builtinPatterns {
		text = p.Regex.ReplaceAllString(text, fmt.Sprintf("[REDACTED:%s]", p.Name))
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
