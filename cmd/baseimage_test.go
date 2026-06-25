package cmd

import "testing"

func TestComposeSubcmdBuilds(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"build", []string{"build"}, true},
		{"up detached", []string{"up", "-d"}, true},
		{"run", []string{"run", "laravel.test", "bash"}, true},
		{"create", []string{"create"}, true},
		{"ps", []string{"ps", "-a"}, false},
		{"logs follow", []string{"logs", "-f"}, false},
		{"down", []string{"down"}, false},
		{"leading file flag then build", []string{"-f", "x", "build"}, true},
		{"empty", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := composeSubcmdBuilds(tc.args); got != tc.want {
				t.Errorf("composeSubcmdBuilds(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
