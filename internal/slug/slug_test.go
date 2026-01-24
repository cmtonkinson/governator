package slug

import "testing"

// TestSlugify covers the core normalization rules used by Governator.
func TestSlugify(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
		{name: "letters only", in: "Governator", want: "governator"},
		{name: "mixed case and digits", in: "Task 42", want: "task-42"},
		{name: "punctuation collapse", in: "Review!!! Phase", want: "review-phase"},
		{name: "trim hyphen", in: "--slug--", want: "slug"},
		{name: "multiple separators", in: "A/B\\C", want: "a-b-c"},
		{name: "retain numbers", in: "Rule 17-99", want: "rule-17-99"},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slugify(tt.in); got != tt.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
