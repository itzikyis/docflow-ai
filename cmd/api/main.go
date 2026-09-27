// Command api is the DocFlow AI HTTP API.
package main

import "fmt"

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("docflow-ai api %s\n", version)
}
