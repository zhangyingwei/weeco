package commands

import (
	"reflect"
	"testing"
)

func TestModelArgs(t *testing.T) {
	cases := []struct {
		kind, model, effort string
		want                []string
	}{
		{"claude", "opus", "high", []string{"--model", "opus", "--effort", "high"}},
		{"claude", "opus", "none", []string{"--model", "opus"}},
		{"codex", "gpt-6", "medium", []string{"-m", "gpt-6", "-c", "model_reasoning_effort=medium"}},
		{"pi", "deepseek", "low", []string{"--model", "deepseek", "--thinking", "low"}},
		{"claude", "", "", nil},
	}
	for _, tc := range cases {
		got, err := modelArgs(tc.kind, tc.model, tc.effort)
		if err != nil {
			t.Fatalf("%s/%s/%s: %v", tc.kind, tc.model, tc.effort, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s/%s/%s = %#v, want %#v", tc.kind, tc.model, tc.effort, got, tc.want)
		}
	}
	if _, err := modelArgs("claude", "x", "ultra"); err == nil {
		t.Fatal("unsupported effort accepted")
	}
	if _, err := modelArgs("gemini", "x", ""); err == nil {
		t.Fatal("unsupported agent accepted")
	}
}
