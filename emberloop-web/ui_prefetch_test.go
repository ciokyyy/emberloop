package web

import (
	"reflect"
	"testing"
)

func TestPrefetchPathsByRoute(t *testing.T) {
	tests := []struct {
		route string
		want  []string
	}{
		{"/", []string{"/api/dashboard", "/api/projects"}},
		{"/s/session-1", []string{"/api/sessions/session-1?tail=40", "/api/users", "/api/projects"}},
		{"/p/project-1/runs", []string{"/api/runs", "/api/projects"}},
		{"/settings/profile", []string{"/api/settings", "/api/projects"}},
		{"/p/project-1", []string{"/api/projects", "/api/sessions"}},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			if got := prefetchPaths(tt.route, "session-1"); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("prefetchPaths(%q) = %#v, want %#v", tt.route, got, tt.want)
			}
		})
	}
}
