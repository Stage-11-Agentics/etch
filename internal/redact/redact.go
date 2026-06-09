package redact

import (
	"github.com/Stage-11-Agentics/etch/internal/config"
)

func Redact(text string, settings config.Settings) string {
	return newRedactor(settings).apply(text)
}
