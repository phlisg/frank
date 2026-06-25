package baseimage

import "testing"

func TestNeedsBuild(t *testing.T) {
	const want = "abc123"

	tests := []struct {
		name     string
		present  bool
		gotLabel string
		want     bool
	}{
		{"absent", false, "", true},
		{"label differs", true, "deadbeef", true},
		{"label empty", true, "", true},
		{"label no value", true, "<no value>", true},
		{"label no value padded", true, "  <no value>  ", true},
		{"label matches", true, want, false},
		{"label matches padded", true, "  abc123  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsBuild(tt.present, tt.gotLabel, want); got != tt.want {
				t.Errorf("needsBuild(%v, %q, %q) = %v, want %v",
					tt.present, tt.gotLabel, want, got, tt.want)
			}
		})
	}
}
