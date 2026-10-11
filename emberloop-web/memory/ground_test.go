package memory

import (
	"errors"
	"testing"
)

func TestCheckGroundingAcceptsUserQuote(t *testing.T) {
	ev := GroundEvent{Found: true, Sender: "user", Text: "please always wrap errors in this repo"}
	if err := CheckGrounding("3", "always wrap errors", ev); err != nil {
		t.Fatalf("grounded quote rejected: %v", err)
	}
}

func TestCheckGroundingRejectsMissingEvent(t *testing.T) {
	err := CheckGrounding("99", "always wrap errors", GroundEvent{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing event err = %v, want ErrNotFound", err)
	}
}

func TestCheckGroundingRejectsSystemSender(t *testing.T) {
	for _, sender := range []string{"system", ""} {
		ev := GroundEvent{Found: true, Sender: sender, Text: "always wrap errors"}
		if err := CheckGrounding("3", "always wrap errors", ev); !errors.Is(err, ErrInvalid) {
			t.Errorf("sender %q err = %v, want ErrInvalid", sender, err)
		}
	}
}

func TestCheckGroundingRejectsNonSubstringQuote(t *testing.T) {
	cases := []struct {
		name  string
		quote string
		text  string
	}{
		{"different words", "always wrap errors", "never wrap errors here"},
		{"case differs", "Always Wrap Errors", "always wrap errors"},
		{"empty quote", "", "always wrap errors"},
		{"quote longer than text", "always wrap errors everywhere", "wrap errors"},
	}
	for _, c := range cases {
		ev := GroundEvent{Found: true, Sender: "user", Text: c.text}
		if err := CheckGrounding("3", c.quote, ev); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", c.name, err)
		}
	}
}
