package capture

import (
	"os"
	"runtime"
	"strings"

	"github.com/Stage-11-Agentics/etch/internal/config"
	"github.com/Stage-11-Agentics/etch/internal/redact"
)

// CaptureMachine reads machine identity from the current system.
// The hostname is stored as a salted hash (per-repo salt, see
// config.EnsureHostnameSalt); when settings.RawMachineIdentity is true,
// the raw hostname is also captured.
func CaptureMachine(settings config.Settings, salt string) MachineInfo {
	hostname, _ := os.Hostname()

	m := MachineInfo{
		HostnameHash: redact.HashHostname(salt, hostname),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
	}

	if settings.RawMachineIdentity {
		raw := hostname
		m.HostnameRaw = &raw
	}

	m.OSVersion = osVersion()
	return m
}

func osVersion() string {
	if runtime.GOOS == "darwin" {
		out, err := execOutput("uname", "-v")
		if err == nil && out != "" {
			return "Darwin " + extractDarwinVersion(out)
		}
	}
	out, err := execOutput("uname", "-r")
	if err == nil {
		return out
	}
	return runtime.GOOS
}

func extractDarwinVersion(unameV string) string {
	// uname -v: "Darwin Kernel Version 25.5.0: ..."
	parts := strings.Fields(unameV)
	for i, p := range parts {
		if p == "Version" && i+1 < len(parts) {
			ver := strings.TrimSuffix(parts[i+1], ":")
			return ver
		}
	}
	return strings.TrimSpace(unameV)
}

func execOutput(name string, args ...string) (string, error) {
	return strings.TrimSpace(gitOutput(".", name, args...)), nil
}
