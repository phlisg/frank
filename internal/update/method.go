package update

import (
	"os"
	"path/filepath"
	"strings"
)

type Method int

const (
	MethodBrew    Method = iota
	MethodGo
	MethodUnknown
)

func (m Method) String() string {
	switch m {
	case MethodBrew:
		return "brew"
	case MethodGo:
		return "go"
	default:
		return "unknown"
	}
}

func DetectMethod() Method {
	return detectFromPath(executablePath())
}

func detectFromPath(path string) Method {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "/cellar/") || strings.Contains(lower, "/homebrew/") {
		return MethodBrew
	}
	if strings.Contains(lower, "/go/bin/") {
		return MethodGo
	}
	return MethodUnknown
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return resolved
}
