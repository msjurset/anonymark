package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/msjurset/anonymark/pkg/renderer"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("could not determine home dir")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde path expansion",
			input:    "~/test.png",
			expected: filepath.Join(home, "test.png"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/tmp/test.png",
			expected: "/tmp/test.png",
		},
		{
			name:     "relative path unchanged",
			input:    "output.png",
			expected: "output.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := expandPath(tt.input)
			if actual != tt.expected {
				t.Errorf("expandPath(%q) = %q, expected %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestParseProcessArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantInput   string
		wantOutput  string
		wantMode    renderer.Mode
	}{
		{
			name:       "default flags",
			args:       []string{"input.png"},
			wantInput:  "input.png",
			wantOutput: "anonymized.png",
			wantMode:   renderer.ModeSynthetic,
		},
		{
			name:       "custom output and mode flags space separated",
			args:       []string{"input.png", "-out", "custom.png", "-mode", "blur"},
			wantInput:  "input.png",
			wantOutput: "custom.png",
			wantMode:   renderer.ModeBlur,
		},
		{
			name:       "custom output and mode flags equals separated",
			args:       []string{"input.png", "--out=out.png", "--mode=pill"},
			wantInput:  "input.png",
			wantOutput: "out.png",
			wantMode:   renderer.ModePill,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, out, mode := parseProcessArgs(tt.args)
			if in != tt.wantInput {
				t.Errorf("input = %q, want %q", in, tt.wantInput)
			}
			if out != tt.wantOutput {
				t.Errorf("output = %q, want %q", out, tt.wantOutput)
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}
