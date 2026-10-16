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
		{"/", []string{"/api/auth/me", "/api/dashboard", "/api/projects"}},
		{"/s/session-1", []string{"/api/auth/me", "/api/sessions/session-1?tail=40", "/api/users", "/api/projects"}},
		{"/p/project-1/runs", []string{"/api/auth/me", "/api/runs", "/api/projects"}},
		{"/settings/profile", []string{"/api/auth/me", "/api/settings", "/api/projects"}},
		{"/p/project-1", []string{"/api/auth/me", "/api/projects", "/api/sessions"}},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			if got := prefetchPaths(tt.route, "session-1"); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("prefetchPaths(%q) = %#v, want %#v", tt.route, got, tt.want)
			}
		})
	}
}
