package redact

import (
	"fmt"

	"forgejo.stage11.ai/s11/etch/internal/config"
)

func Redact(text string, settings config.Settings) string {
	text = ScanSecrets(text)

	custom := compileCustomPatterns(settings.RedactionPatterns)
	for _, p := range custom {
		text = p.Regex.ReplaceAllString(text, fmt.Sprintf("[REDACTED:%s]", p.Name))
	}

	return text
}
