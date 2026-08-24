package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/CJhuochai/project-brain/internal/workspace"
)

func Run(args []string, output io.Writer) error {
	if len(args) != 2 || args[0] != "discover" {
		return fmt.Errorf("usage: project-brain discover <workspace>")
	}
	repositories, err := workspace.Discover(args[1])
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(repositories)
}
