package redact

import (
	"forgejo.stage11.ai/s11/etch/internal/config"
)

func Redact(text string, settings config.Settings) string {
	return newRedactor(settings).apply(text)
}
