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

// ETCH-28: the whole PEM block — material and END line — must be redacted,
// not just the BEGIN header.
func TestScanSecretsPrivateKeyFullBlock(t *testing.T) {
	material1 := "MIIEpAIBAAKCAQEA7examplekeymaterial1234567890abcdefghijklmnopqr"
	material2 := "stuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789exampleexampleexamp"
	tests := []struct {
		name  string
		input string
	}{
		{"RSA block", "-----BEGIN RSA PRIVATE KEY-----\n" + material1 + "\n" + material2 + "\n-----END RSA PRIVATE KEY-----"},
		{"PKCS8 block", "-----BEGIN PRIVATE KEY-----\n" + material1 + "\n-----END PRIVATE KEY-----"},
		{"OPENSSH block", "-----BEGIN OPENSSH PRIVATE KEY-----\n" + material1 + "\n-----END OPENSSH PRIVATE KEY-----"},
		{"ENCRYPTED block", "-----BEGIN ENCRYPTED PRIVATE KEY-----\n" + material1 + "\n-----END ENCRYPTED PRIVATE KEY-----"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets("before\n" + tt.input + "\nafter")
			if !strings.Contains(out, "[REDACTED:private-key]") {
				t.Fatalf("block not redacted: %s", out)
			}
			if strings.Contains(out, material1) || strings.Contains(out, material2) {
				t.Errorf("key MATERIAL leaked: %s", out)
			}
			if strings.Contains(out, "-----END") {
				t.Errorf("END line leaked: %s", out)
			}
			if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
				t.Errorf("surrounding text clobbered: %s", out)
			}
		})
	}
}

// Truncated block (no END marker): header plus material lines still redact.
func TestScanSecretsPrivateKeyTruncated(t *testing.T) {
	material := "MIIEpAIBAAKCAQEA7examplekeymaterial1234567890abcdefghijklmnopqr"
	input := "-----BEGIN RSA PRIVATE KEY-----\n" + material + "\nand then prose continues"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:private-key]") {
		t.Fatalf("truncated block not redacted: %s", out)
	}
	if strings.Contains(out, material) {
		t.Errorf("truncated key material leaked: %s", out)
	}
	if !strings.Contains(out, "and then prose continues") {
		t.Errorf("prose after truncated block clobbered: %s", out)
	}
}

// ETCH-27: bare JWTs (three base64url segments) must be redacted.
func TestScanSecretsJWT(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ticket repro", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_part"},
		{"in prose", "the token eyJhbGciOiJSUzI1NiIsImtpZCI6ImtleTEifQ.eyJpc3MiOiJodHRwczovL2V4YW1wbGUifQ.dGhpc2lzYXNpZ25hdHVyZQ ended the line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:jwt]") {
				t.Errorf("JWT not redacted: %s", out)
			}
			if strings.Contains(out, "eyJ") {
				t.Errorf("JWT content leaked: %s", out)
			}
		})
	}
}

func TestScanSecretsJWTNegative(t *testing.T) {
	inputs := []string{
		"x.y.z",
		"version 1.2.3 released",
		"see docs.example.com today",
		"eyJshort.x.y", // segments below minimum length
	}
	for _, input := range inputs {
		out := ScanSecrets(input)
		if out != input {
			t.Errorf("non-JWT clobbered: %q → %q", input, out)
		}
	}
}

// ETCH-40 finding 7: modern prefixed OpenAI keys.
func TestScanSecretsOpenAIModern(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"project key", "key is sk-proj-AbCdEf123456_789-abcdefGHIJKL"},
		{"service account key", "sk-svcacct-AbCdEf123456_789-abcdefGHIJKL"},
		{"admin key", "sk-admin-AbCdEf123456_789-abcdefGHIJKL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:openai-api-key]") {
				t.Errorf("modern OpenAI key not redacted: %s", out)
			}
			if strings.Contains(out, "AbCdEf123456") {
				t.Errorf("key body leaked: %s", out)
			}
		})
	}
}

// ETCH-29: documentation placeholders must NOT be redacted.
func TestScanSecretsPlaceholdersPreserved(t *testing.T) {
	inputs := []string{
		"use sk-ant-EXAMPLE as a placeholder",
		"sk-ant-PLACEHOLDER",
		"sk-DOCUMENTATION-NOT-A-KEY",
		"sk-proj-EXAMPLE", // body too short to be real
		"sk-ant-yourkeyhere",
	}
	for _, input := range inputs {
		out := ScanSecrets(input)
		if out != input {
			t.Errorf("placeholder clobbered: %q → %q", input, out)
		}
	}
}

// Real-shape anthropic keys (tier segment + long body) still redact.
func TestScanSecretsAnthropicRealShapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"api key", "ANTHROPIC_API_KEY=sk-ant-api03-AbCd1234efGh5678IjKl9012MnOp"},
		{"oauth token", "sk-ant-oat01-AbCd1234efGh5678IjKl9012MnOp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:anthropic-api-key]") {
				t.Errorf("anthropic key not redacted: %s", out)
			}
			if strings.Contains(out, "AbCd1234efGh") {
				t.Errorf("key body leaked: %s", out)
			}
		})
	}
}

// ETCH-26: bare AWS secret access keys (no key= label) must be redacted.
func TestScanSecretsBareAWSSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ticket repro bare", "the key wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY appeared"},
		{"start of string", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:aws-secret-key]") {
				t.Errorf("bare AWS secret not redacted: %s", out)
			}
			if strings.Contains(out, "wJalrXUtnFEMI") {
				t.Errorf("secret leaked: %s", out)
			}
		})
	}
}

// Structural fields that flow through DeepRedact must NOT trip the bare-AWS
// pattern: git SHAs, sha256 hashes, ULIDs, long base64 blobs.
func TestScanSecretsBareAWSSecretNegative(t *testing.T) {
	inputs := []string{
		"01a2ca4f3e8b9d2c5a7f1e6b4d8c3a9f2e5b7d1c",                          // 40-char lowercase git SHA
		"sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", // 64-hex
		"01ARZ3NDEKTSV4RRFFQ69G5FAV",                                        // ULID (26 chars)
		"dGhpc2lzYWxvbmdlcmJhc2U2NGJsb2J0aGF0Z29lc29uYW5kb24=",              // base64 blob ≠ 40 chars
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",                          // 40 chars but no lower/digit
	}
	for _, input := range inputs {
		out := ScanSecrets(input)
		if out != input {
			t.Errorf("structural value clobbered: %q → %q", input, out)
		}
	}
}

// Documented, accepted miss: a 40-char secret embedded inside a LONGER
// contiguous base64 run is not caught (the maximal run fails len==40).
// This pins the best-effort tradeoff so it reads as intentional.
func TestScanSecretsBareAWSSecretKnownMiss(t *testing.T) {
	input := "prefix00" + "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" // one 48-char run
	out := ScanSecrets(input)
	if out != input {
		t.Errorf("expected known miss to pass through, got: %q", out)
	}
}

// Documented, accepted false positive: a PATH whose base64-ish run (slashes
// included) is exactly 40 chars trips the bare-AWS shape — observed with
// "etch/sessions/<26-char ULID>" (14+26=40). Over-redaction of path metadata
// is the safe failure direction; this pins the behavior as known.
func TestScanSecretsBareAWSSecretPathFP(t *testing.T) {
	input := ".etch/sessions/01KTH9VSXVVAJMQTDJRDEWQQPB.wip.jsonl"
	out := ScanSecrets(input)
	if !strings.Contains(out, "[REDACTED:aws-secret-key]") {
		t.Logf("note: exact-40 path FP no longer occurs (acceptable precision improvement): %q", out)
	}
}

// ETCH-39: common credential keywords.
func TestScanSecretsCredentialKeywords(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"password equals", "password=SuperSecret123"},
		{"passwd colon", "passwd: SuperSecret123"},
		{"pwd equals", "pwd=SuperSecret123"},
		{"bare token", "token = abcdef1234567890"},
		{"client_secret", "client_secret=abcdef1234567890"},
		{"env DB_PASS", "DB_PASS=hunter2password"},
		{"env AWS_SECRET", "AWS_SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
		{"env SECRET", "SECRET=abcdef1234567890"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ScanSecrets(tt.input)
			if !strings.Contains(out, "[REDACTED:") {
				t.Errorf("credential not redacted: %s", out)
			}
		})
	}
}

// Precision/recall pinning: keyword must be immediately followed by : or =,
// so token-count prose survives; compass= is a documented, accepted FP.
func TestScanSecretsCredentialKeywordsNegative(t *testing.T) {
	preserved := []string{
		"tokens: 4096",
		"max_tokens=8192",
		"passwords are stored hashed", // no [:=] after keyword
		"pwd is /home/user",           // value too short
	}
	for _, input := range preserved {
		out := ScanSecrets(input)
		if out != input {
			t.Errorf("prose clobbered: %q → %q", input, out)
		}
	}

	// Documented accepted false positive: a keyword suffix inside a longer
	// identifier still matches (best-effort regex, leak-averse direction).
	fp := "compass=abcdef123456"
	if out := ScanSecrets(fp); out == fp {
		t.Logf("note: compass= no longer a false positive (acceptable)")
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
