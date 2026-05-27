package capture

import (
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// CaptureMachine reads machine identity from the current system.
func CaptureMachine() MachineInfo {
	hostname, _ := os.Hostname()
	hash := sha256.Sum256([]byte(hostname))

	m := MachineInfo{
		HostnameHash: fmt.Sprintf("sha256:%x", hash),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
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
