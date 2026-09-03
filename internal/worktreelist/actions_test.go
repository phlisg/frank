package worktreelist

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWebPort_HTTPS(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Publishers: []Publisher{
				{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
				{TargetPort: 443, PublishedPort: 32770, Protocol: "udp"},
				{TargetPort: 5173, PublishedPort: 5173, Protocol: "tcp"},
			}},
		},
	}
	if got := item.WebPort(); got != 32771 {
		t.Errorf("WebPort() = %d, want 32771 (443/tcp)", got)
	}
}

func TestWebPort_HTTP(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Publishers: []Publisher{
				{TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
			}},
		},
	}
	if got := item.WebPort(); got != 8080 {
		t.Errorf("WebPort() = %d, want 8080 (80/tcp)", got)
	}
}

func TestWebPort_PrefersHTTPS(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Publishers: []Publisher{
				{TargetPort: 80, PublishedPort: 8080, Protocol: "tcp"},
				{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
			}},
		},
	}
	if got := item.WebPort(); got != 32771 {
		t.Errorf("WebPort() = %d, want 32771 (443/tcp preferred over 80/tcp)", got)
	}
}

func TestWebPort_SkipsUDP(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Publishers: []Publisher{
				{TargetPort: 443, PublishedPort: 32770, Protocol: "udp"},
			}},
		},
	}
	if got := item.WebPort(); got != 0 {
		t.Errorf("WebPort() = %d, want 0 (only UDP available)", got)
	}
}

func TestWebPort_NoLaravelTest(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "pgsql", State: "running"},
		},
	}
	if got := item.WebPort(); got != 0 {
		t.Errorf("WebPort() = %d, want 0", got)
	}
}

func TestWebPort_NoPorts(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running"},
		},
	}
	if got := item.WebPort(); got != 0 {
		t.Errorf("WebPort() = %d, want 0", got)
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		name string
		item WorktreeItem
		want string
	}{
		{
			"not configured",
			WorktreeItem{HasFrank: false},
			"not configured",
		},
		{
			"stopped no services",
			WorktreeItem{HasFrank: true},
			"stopped",
		},
		{
			"stopped all exited",
			WorktreeItem{HasFrank: true, Services: []ServiceInfo{
				{State: "exited"}, {State: "exited"},
			}},
			"stopped",
		},
		{
			"running all",
			WorktreeItem{HasFrank: true, Services: []ServiceInfo{
				{State: "running"}, {State: "running"},
			}},
			"running (2/2)",
		},
		{
			"partial",
			WorktreeItem{HasFrank: true, Services: []ServiceInfo{
				{State: "running"}, {State: "exited"},
			}},
			"partial (1/2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.StatusLabel(); got != tt.want {
				t.Errorf("StatusLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPortSummary(t *testing.T) {
	tests := []struct {
		name string
		item WorktreeItem
		want string
	}{
		{
			"labels and skips non-running",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "laravel.test", State: "running", Publishers: []Publisher{
					{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
				}},
				{Name: "pgsql", State: "running", Publishers: []Publisher{
					{TargetPort: 5432, PublishedPort: 32768, Protocol: "tcp"},
				}},
				{Name: "redis", State: "exited", Publishers: []Publisher{
					{TargetPort: 6379, PublishedPort: 6379, Protocol: "tcp"},
				}},
			}},
			"web:32771  pgsql:32768",
		},
		{
			"skips udp publishers",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "laravel.test", State: "running", Publishers: []Publisher{
					{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
					{TargetPort: 443, PublishedPort: 32770, Protocol: "udp"},
				}},
			}},
			"web:32771",
		},
		{
			"collapses multiple ports",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "laravel.test", State: "running", Publishers: []Publisher{
					{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
					{TargetPort: 80, PublishedPort: 32770, Protocol: "tcp"},
				}},
			}},
			"web:32771,32770",
		},
		{
			"orders web, vite, then alphabetical",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "gotenberg", State: "running", Publishers: []Publisher{
					{TargetPort: 3000, PublishedPort: 32790, Protocol: "tcp"},
				}},
				{Name: "pgsql", State: "running", Publishers: []Publisher{
					{TargetPort: 5432, PublishedPort: 32768, Protocol: "tcp"},
				}},
				{Name: "laravel.vite", State: "running", Publishers: []Publisher{
					{TargetPort: 5173, PublishedPort: 32773, Protocol: "tcp"},
				}},
				{Name: "laravel.test", State: "running", Publishers: []Publisher{
					{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
				}},
			}},
			"web:32771  vite:32773  gotenberg:32790  pgsql:32768",
		},
		{
			"omits services with no published ports",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "laravel.test", State: "running", Publishers: []Publisher{
					{TargetPort: 443, PublishedPort: 32771, Protocol: "tcp"},
				}},
				{Name: "memcached", State: "running"},
			}},
			"web:32771",
		},
		{
			"no published ports at all",
			WorktreeItem{Services: []ServiceInfo{
				{Name: "laravel.test", State: "running"},
			}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.PortSummary(); got != tt.want {
				t.Errorf("PortSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	running := WorktreeItem{
		Services: []ServiceInfo{{State: "running"}},
	}
	if !running.IsRunning() {
		t.Error("expected IsRunning()=true")
	}

	stopped := WorktreeItem{
		Services: []ServiceInfo{{State: "exited"}},
	}
	if stopped.IsRunning() {
		t.Error("expected IsRunning()=false")
	}

	empty := WorktreeItem{}
	if empty.IsRunning() {
		t.Error("expected IsRunning()=false for empty services")
	}
}

// lineWriter must split on both \n and \r (compose redraws in place with \r)
// and strip ANSI so the status line never carries escape codes into the list.
func TestLineWriterSplitsAndStripsANSI(t *testing.T) {
	var got []string
	w := &lineWriter{send: func(msg tea.Msg) {
		got = append(got, msg.(progressMsg).line)
	}}

	_, _ = w.Write([]byte("\x1b[32m✓ Starting\x1b[0m\n[+] Running 1/2\r"))
	_, _ = w.Write([]byte("  partial"))

	want := []string{"✓ Starting", "[+] Running 1/2"}
	if len(got) != len(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}
