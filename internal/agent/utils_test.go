package agent

import (
	"reflect"
	"testing"
)

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"yes", true},
		{"YES", true},
		{"Yes", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{" 1 ", true},
		{"no", false},
		{"false", false},
		{"0", false},
		{"", false},
		{"  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isTruthy(tt.input); got != tt.want {
				t.Errorf("isTruthy(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"all same", []string{"x", "x", "x"}, []string{"x"}},
		{"empty slice", []string{}, []string{}},
		{"single element", []string{"a"}, []string{"a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeDuplicates(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeDuplicates(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPrettyJSON(t *testing.T) {
	input := map[string]string{"key": "value"}
	got, err := prettyJSON(input)
	if err != nil {
		t.Fatalf("prettyJSON returned error: %v", err)
	}
	expected := "{\n  \"key\": \"value\"\n}"
	if got != expected {
		t.Errorf("prettyJSON() = %q, want %q", got, expected)
	}
}
