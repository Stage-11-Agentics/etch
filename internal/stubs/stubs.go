package stubs

import (
	"fmt"
	"io"
	"os"
)

func Run() error {
	if !isTerminal(os.Stdin) {
		io.Copy(io.Discard, os.Stdin)
	}
	fmt.Println(`{"ok":true}`)
	return nil
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
