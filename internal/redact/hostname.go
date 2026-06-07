package redact

import (
	"crypto/sha256"
	"fmt"
	"os"

	"forgejo.stage11.ai/s11/etch/internal/config"
)

// HashHostname is the canonical hostname-hash derivation:
// sha256:hex(SHA-256(salt + hostname)). The salt is the per-repo value from
// .etch/settings.json (config.EnsureHostnameSalt); an empty salt degrades to
// an unsalted hash rather than failing.
func HashHostname(salt, hostname string) string {
	h := sha256.Sum256([]byte(salt + hostname))
	return fmt.Sprintf("sha256:%x", h)
}

type HostnameResult struct {
	Hash string
	Raw  string
}

func GetHostname(settings config.Settings, salt string) HostnameResult {
	hostname, _ := os.Hostname()
	result := HostnameResult{
		Hash: HashHostname(salt, hostname),
	}
	if settings.RawMachineIdentity {
		result.Raw = hostname
	}
	return result
}
