package update

import (
	"strings"
	"testing"
)

type mockCommander struct {
	calls []string
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return nil
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		method   Method
		latest   string
		wantCall string
	}{
		{"brew", MethodBrew, "2.0.0", "brew upgrade frank"},
		{"go", MethodGo, "1.2.3", "go install github.com/phlisg/frank@v1.2.3"},
		{"unknown", MethodUnknown, "1.0.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockCommander{}
			cmd = mock

			// Override DetectMethod by calling Run directly with a patched detect
			// We need to test through Run, so we'll use a helper approach
			err := runWithMethod(tt.method, tt.latest)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantCall == "" {
				if len(mock.calls) != 0 {
					t.Fatalf("expected no calls, got %v", mock.calls)
				}
				return
			}

			if len(mock.calls) != 1 {
				t.Fatalf("expected 1 call, got %d: %v", len(mock.calls), mock.calls)
			}
			if mock.calls[0] != tt.wantCall {
				t.Errorf("got %q, want %q", mock.calls[0], tt.wantCall)
			}
		})
	}
}
