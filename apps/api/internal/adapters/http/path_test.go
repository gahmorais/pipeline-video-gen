package httpapi

import "testing"

func TestJobIDFromPath(t *testing.T) {
	tests := map[string]string{
		"/videos/vj_1":                          "vj_1",
		"/videos/vj_1/events":                   "vj_1",
		"/internal/videos/vj_2/renders/r/start": "vj_2",
		"/healthz":                              "",
		"/videos/":                              "",
		"/videos/..%2F":                         "",
	}
	for path, want := range tests {
		if got := jobIDFromPath(path); got != want {
			t.Errorf("jobIDFromPath(%q) = %q, esperado %q", path, got, want)
		}
	}
}
