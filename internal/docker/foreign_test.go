package docker

import "testing"

func TestForeignComposeFile(t *testing.T) {
	own := "/home/u/proj/.frank/compose.yaml"

	tests := []struct {
		name        string
		configFiles string
		want        string
	}{
		{"frank's own file", own, ""},
		{"own file among several", "/home/u/proj/compose.override.yaml," + own, ""},
		{"unclean own path", "/home/u/proj/.frank/../.frank/compose.yaml", ""},
		{"padded", "  " + own + "  ", ""},
		{"no container running", "", ""},
		{"sail", "/home/u/proj/docker-compose.yml", "/home/u/proj/docker-compose.yml"},
	}

	for _, tt := range tests {
		if got := foreignComposeFile(tt.configFiles, own); got != tt.want {
			t.Errorf("%s: foreignComposeFile(%q) = %q, want %q", tt.name, tt.configFiles, got, tt.want)
		}
	}
}
