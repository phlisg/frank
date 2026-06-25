package worktreelist

import "testing"

func TestWebPort_HTTPS(t *testing.T) {
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Ports: ":32771 :32770 :5173", Publishers: []Publisher{
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
			{Name: "laravel.test", State: "running", Ports: ":8080", Publishers: []Publisher{
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
			{Name: "laravel.test", State: "running", Ports: ":32771 :8080", Publishers: []Publisher{
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
			{Name: "laravel.test", State: "running", Ports: ":32770", Publishers: []Publisher{
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
			{Name: "pgsql", State: "running", Ports: ":5432"},
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
	item := WorktreeItem{
		Services: []ServiceInfo{
			{Name: "laravel.test", State: "running", Ports: ":443 :5173"},
			{Name: "pgsql", State: "running", Ports: ":5432"},
			{Name: "redis", State: "exited", Ports: ":6379"},
		},
	}
	got := item.PortSummary()
	want := ":443 :5173 :5432"

	if got != want {
		t.Errorf("PortSummary() = %q, want %q", got, want)
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
