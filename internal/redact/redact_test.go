package redact

import (
	"strings"
	"testing"

	"forgejo.stage11.ai/s11/etch/internal/config"
)

func TestHashHostname(t *testing.T) {
	h1 := HashHostname("myhost")
	h2 := HashHostname("myhost")
	if h1 != h2 {
		t.Error("HashHostname should be deterministic")
	}
	if !strings.HasPrefix(h1, "sha256:") {
		t.Errorf("hash should start with sha256:, got %s", h1)
	}
	if len(h1) != len("sha256:")+64 {
		t.Errorf("hash should be sha256: + 64 hex chars, got len %d", len(h1))
	}

	h3 := HashHostname("otherhost")
	if h1 == h3 {
		t.Error("different hostnames should produce different hashes")
	}
}

func TestGetHostnameDefault(t *testing.T) {
	s := config.Defaults()
	result := GetHostname(s)
	if result.Hash == "" {
		t.Error("hash should never be empty")
	}
	if result.Raw != "" {
		t.Error("raw should be empty when RawMachineIdentity is false")
	}
}

func TestGetHostnameRawOptIn(t *testing.T) {
	s := config.Settings{RawMachineIdentity: true}
	result := GetHostname(s)
	if result.Hash == "" {
		t.Error("hash should always be populated")
	}
	if result.Raw == "" {
		t.Error("raw should be populated when RawMachineIdentity is true")
	}
}

func TestScanSecretsAWS(t *testing.T) {
	input := "key is AKIAIOSFODNN7EXAMPLE"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:aws-access-key]") {
		t.Errorf("AWS key not redacted: %s", out)
	}
	if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("original AWS key still present")
	}
}

func TestScanSecretsAWSSecretKey(t *testing.T) {
	input := "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:aws-secret-key]") {
		t.Errorf("AWS secret key not redacted: %s", out)
	}
}

func TestScanSecretsAnthropic(t *testing.T) {
	input := "export ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnop"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:anthropic-api-key]") {
		t.Errorf("Anthropic key not redacted: %s", out)
	}
}

func TestScanSecretsOpenAI(t *testing.T) {
	input := "key=sk-abcdefghijklmnopqrstuvwxyz1234567890"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:openai-api-key]") {
		t.Errorf("OpenAI key not redacted: %s", out)
	}
}

func TestScanSecretsStripeLive(t *testing.T) {
	input := "stripe key: sk_live_abcdefghijklmnop"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:stripe-live-key]") {
		t.Errorf("Stripe live key not redacted: %s", out)
	}
}

func TestScanSecretsStripeTest(t *testing.T) {
	input := "stripe key: sk_test_abcdefghijklmnop"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:stripe-test-key]") {
		t.Errorf("Stripe test key not redacted: %s", out)
	}
}

func TestScanSecretsGeneric(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"api_key equals", `api_key = "abcdefghijklmnop1234"`},
		{"api-key colon", `api-key: abcdefghijklmnop1234`},
		{"apikey equals", `apikey=abcdefghijklmnop1234`},
		{"api_secret", `api_secret = abcdefghijklmnop1234`},
		{"access_token", `access_token: abcdefghijklmnop1234`},
		{"secret_key", `secret_key = abcdefghijklmnop1234`},
		{"API_KEY upper", `API_KEY=abcdefghijklmnop1234`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:generic-secret]") {
				t.Errorf("generic secret not redacted: %s", out)
			}
		})
	}
}

func TestScanSecretsBearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:bearer-token]") {
		t.Errorf("bearer token not redacted: %s", out)
	}
}

func TestScanSecretsPrivateKey(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAK..."
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:private-key]") {
		t.Errorf("private key not redacted: %s", out)
	}

	input2 := "-----BEGIN PRIVATE KEY-----\nMIIEpAIBAAK..."
	out2 := ScanSecrets(input2)
	if !strings.Contains(out2, "[REDACTED:private-key]") {
		t.Errorf("generic private key not redacted: %s", out2)
	}

	input3 := "-----BEGIN EC PRIVATE KEY-----\nMIIEpAIBAAK..."
	out3 := ScanSecrets(input3)
	if !strings.Contains(out3, "[REDACTED:private-key]") {
		t.Errorf("EC private key not redacted: %s", out3)
	}
}

func TestScanSecretsPassthrough(t *testing.T) {
	inputs := []string{
		"just a normal string",
		"this has no secrets at all",
		"sk- is too short to match",
		"AKIA is too short",
		"api_key = short",
	}
	for _, input := range inputs {
		out := ScanSecrets(input)
		if out != input {
			t.Errorf("non-secret should pass through unchanged: %q → %q", input, out)
		}
	}
}

func TestScanSecretsMultiple(t *testing.T) {
	input := "keys: AKIAIOSFODNN7EXAMPLE and sk-ant-api03-abcdefghijklmnop"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:aws-access-key]") {
		t.Error("first secret not redacted")
	}
	if !strings.Contains(out, "[REDACTED:anthropic-api-key]") {
		t.Error("second secret not redacted")
	}
}

func TestRedactWithCustomPatterns(t *testing.T) {
	s := config.Settings{
		RedactionPatterns: []string{`MYTOKEN-[A-Z0-9]{8}`},
	}
	input := "token is MYTOKEN-ABCD1234 here"
	out := Redact(input, s)
	if !strings.Contains(out, "[REDACTED:custom-0]") {
		t.Errorf("custom pattern not applied: %s", out)
	}
	if strings.Contains(out, "MYTOKEN-ABCD1234") {
		t.Error("original token still present")
	}
}

func TestRedactInvalidCustomPattern(t *testing.T) {
	s := config.Settings{
		RedactionPatterns: []string{`[invalid`},
	}
	input := "nothing to redact"
	out := Redact(input, s)
	if out != input {
		t.Errorf("invalid pattern should be skipped, got: %s", out)
	}
}

func TestRedactCombinesBuiltinAndCustom(t *testing.T) {
	s := config.Settings{
		RedactionPatterns: []string{`CORP-[0-9]{6}`},
	}
	input := "aws: AKIAIOSFODNN7EXAMPLE corp: CORP-123456"
	out := Redact(input, s)
	if !strings.Contains(out, "[REDACTED:aws-access-key]") {
		t.Error("builtin pattern not applied")
	}
	if !strings.Contains(out, "[REDACTED:custom-0]") {
		t.Error("custom pattern not applied")
	}
}

func TestCompileCustomPatterns(t *testing.T) {
	patterns := []string{`abc\d+`, `[invalid`, `def\w+`}
	compiled := compileCustomPatterns(patterns)
	if len(compiled) != 2 {
		t.Errorf("expected 2 valid patterns, got %d", len(compiled))
	}
	if compiled[0].Name != "custom-0" {
		t.Errorf("first pattern name = %s, want custom-0", compiled[0].Name)
	}
	if compiled[1].Name != "custom-2" {
		t.Errorf("second pattern name = %s, want custom-2", compiled[1].Name)
	}
}
