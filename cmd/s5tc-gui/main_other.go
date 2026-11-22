//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "s5tc-gui is only supported on Windows")
	os.Exit(1)
}
