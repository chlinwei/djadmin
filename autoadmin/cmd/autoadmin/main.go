package main

import (
	"fmt"
	"os"

	"autoadmin/internal/app"
	"autoadmin/internal/buildinfo"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("autoadmin %s\n", buildinfo.Version)
		return
	}
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
