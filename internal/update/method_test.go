package update

import "testing"

func TestDetectFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Method
	}{
		{"homebrew intel", "/usr/local/Cellar/frank/1.0.0/bin/frank", MethodBrew},
		{"homebrew apple silicon", "/opt/homebrew/Cellar/frank/1.0.0/bin/frank", MethodBrew},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/frank/1.0.0/bin/frank", MethodBrew},
		{"homebrew bin symlink", "/opt/homebrew/bin/frank", MethodBrew},
		{"go install default", "/home/user/go/bin/frank", MethodGo},
		{"go install root", "/root/go/bin/frank", MethodGo},
		{"unknown usr local", "/usr/local/bin/frank", MethodUnknown},
		{"unknown tmp", "/tmp/frank", MethodUnknown},
		{"empty path", "", MethodUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFromPath(tt.path)
			if got != tt.want {
				t.Errorf("detectFromPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
