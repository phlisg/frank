package update

import (
	"fmt"
	"os"
	"os/exec"
)

type commander interface {
	Run(name string, args ...string) error
}

type execCommander struct{}

func (e *execCommander) Run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

var cmd commander = &execCommander{}

func Run(latest string) error {
	return runWithMethod(DetectMethod(), latest)
}

func runWithMethod(m Method, latest string) error {
	switch m {
	case MethodBrew:
		return cmd.Run("brew", "upgrade", "frank")
	case MethodGo:
		return cmd.Run("go", "install", "github.com/phlisg/frank@v"+latest)
	default:
		fmt.Printf("Download the latest release: https://github.com/phlisg/frank/releases/tag/v%s\n", latest)
		return nil
	}
}
