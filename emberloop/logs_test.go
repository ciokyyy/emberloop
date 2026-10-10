package el

import (
	"reflect"
	"testing"
)

func TestLogsArgs(t *testing.T) {
	got, err := logsArgs([]string{"-n", "5", "-f", "--since", "10m ago", "--grep", "restart"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--user", "-u", "emberloop-web", "--no-pager", "-n", "5", "-f", "--since", "10m ago", "--grep", "restart"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	def, _ := logsArgs(nil)
	if def[len(def)-1] != "50" {
		t.Fatalf("default lines: %v", def)
	}
	if _, err := logsArgs([]string{"extra"}); err == nil {
		t.Fatal("want usage error for positional arg")
	}
}
