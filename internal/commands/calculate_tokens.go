package commands

import (
	"encoding/json"
	"fmt"
)

func RunCalculateTokens(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: calculate-tokens <session-id>")
	}

	session, err := readSessionFromRef(args[0])
	if err != nil {
		return err
	}

	tokens, ok := session["tokens"]
	if !ok || tokens == nil {
		fmt.Println(`{"input":0,"output":0,"cache_read":0,"cache_write":0,"api_calls":0,"estimated_cost_usd":0}`)
		return nil
	}

	out, _ := json.MarshalIndent(tokens, "", "  ")
	fmt.Println(string(out))
	return nil
}
