// Command sphinxor is Sphinxor's CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/chebilax/sphinxor/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
