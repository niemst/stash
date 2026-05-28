package brain

import "testing"

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/foo_bar", `/foo\_bar`},
		{"/foo/bar", `/foo/bar`},
		{"/100%", `/100\%`},
		{`/a\b`, `/a\\b`},
		{"/foo_bar/baz_qux", `/foo\_bar/baz\_qux`},
	}
	for _, tt := range tests {
		if got := escapeLikePattern(tt.in); got != tt.want {
			t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLikePatternForDescendants(t *testing.T) {
	got := likePatternForDescendants("/foo_bar")
	want := `/foo\_bar/%`
	if got != want {
		t.Errorf("likePatternForDescendants(/foo_bar) = %q, want %q", got, want)
	}
}
