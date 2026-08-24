package main

import (
	"fmt"
	"os"

	"github.com/CJhuochai/project-brain/internal/app"
	"github.com/CJhuochai/project-brain/internal/mcp"
)

const version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "mcp" {
		if err := mcp.Serve(os.Stdin, os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if err := app.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
