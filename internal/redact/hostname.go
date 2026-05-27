package redact

import (
	"crypto/sha256"
	"fmt"
	"os"

	"forgejo.stage11.ai/s11/etch/internal/config"
)

func HashHostname(hostname string) string {
	h := sha256.Sum256([]byte(hostname))
	return fmt.Sprintf("sha256:%x", h)
}

type HostnameResult struct {
	Hash string
	Raw  string
}

func GetHostname(settings config.Settings) HostnameResult {
	hostname, _ := os.Hostname()
	result := HostnameResult{
		Hash: HashHostname(hostname),
	}
	if settings.RawMachineIdentity {
		result.Raw = hostname
	}
	return result
}
