package detector

import (
	"testing"
)

func TestDetectorMatches(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType TargetType
		wantContains string
	}{
		{
			name:         "IPv4 Detection",
			input:        "Connecting to 192.168.86.55 on port 22",
			expectedType: TypeIPv4,
			wantContains: "10.0.",
		},
		{
			name:         "MAC Detection",
			input:        "Device D8:3A:DD:4C:F2:BB is online",
			expectedType: TypeMAC,
			wantContains: "52:54:00:",
		},
		{
			name:         "Hostname Detection",
			input:        "Host pi.hole resolved cleanly",
			expectedType: TypeHostname,
			wantContains: ".hole",
		},
		{
			name:         "User Path Detection",
			input:        "Path /Users/msjurseth/workspace/swift",
			expectedType: TypeUserPath,
			wantContains: "developer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDetector()
			matches := d.DetectMatches(tt.input)
			if len(matches) == 0 {
				t.Fatalf("expected match for input %q, got 0", tt.input)
			}
			m := matches[0]
			if m.Type != tt.expectedType {
				t.Errorf("expected type %v, got %v", tt.expectedType, m.Type)
			}
			if !testing.Short() && m.Replacement == "" {
				t.Errorf("replacement should not be empty")
			}
		})
	}
}

func TestDetectorConsistency(t *testing.T) {
	d := NewDetector()
	m1 := d.DetectMatches("Server 192.168.86.55")
	m2 := d.DetectMatches("Client 192.168.86.55")

	if len(m1) == 0 || len(m2) == 0 {
		t.Fatalf("expected matches in both calls")
	}

	if m1[0].Replacement != m2[0].Replacement {
		t.Errorf("expected consistent replacement for same IP, got %q vs %q", m1[0].Replacement, m2[0].Replacement)
	}
}
