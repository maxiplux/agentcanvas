// Command agent-canvas runs the AgentCanvas studio server and its CLI.
package main

import (
	"os"

	"agentcanvas/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
