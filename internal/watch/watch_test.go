package watch

import "testing"

func TestNew(t *testing.T) {
	cfg := Config{
		ProjectRoot:       "/tmp/fake-project",
		ScheduleEnabled:   true,
		QueueCount:        2,
		DockerComposeFile: ".frank/compose.yaml",
	}

	w, err := New(cfg)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if w == nil {
		t.Fatal("New() returned nil watcher")
	}
}
