package mcp

type dockerClient interface {
	RunQuiet(args ...string) (string, error)
	ExecQuiet(service string, command ...string) (string, error)
}
