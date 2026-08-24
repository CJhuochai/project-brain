package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: project-brain --version")
}
