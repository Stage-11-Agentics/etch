package commands

import (
	"encoding/json"
	"fmt"
)

func RunExtractModifiedFiles(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: extract-modified-files <session-id>")
	}

	session, err := readSessionFromRef(args[0])
	if err != nil {
		return err
	}

	type fileEntry struct {
		Path   string `json:"path"`
		Action string `json:"action"`
	}

	var files []fileEntry
	if ft, ok := session["files_touched"].([]any); ok {
		for _, f := range ft {
			if m, ok := f.(map[string]any); ok {
				fe := fileEntry{}
				if p, ok := m["path"].(string); ok {
					fe.Path = p
				}
				if a, ok := m["action"].(string); ok {
					fe.Action = a
				}
				files = append(files, fe)
			}
		}
	}

	out, _ := json.MarshalIndent(files, "", "  ")
	fmt.Println(string(out))
	return nil
}
