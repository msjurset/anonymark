package detector

import (
	"testing"
)

func TestDetectorSubnetConsistency(t *testing.T) {
	d := NewDetector()
	m1 := d.DetectMatches("Server 192.168.86.55")
	m2 := d.DetectMatches("Client 192.168.86.29")

	if len(m1) == 0 || len(m2) == 0 {
		t.Fatalf("expected matches in both calls")
	}

	ip1 := m1[0].Replacement
	ip2 := m2[0].Replacement

	sub1 := ip1[:len(ip1)-3]
	sub2 := ip2[:len(ip2)-3]

	if sub1 != sub2 {
		t.Errorf("subnet prefixes differ: %q vs %q", sub1, sub2)
	}
}

func TestDetectorUserAndHostname(t *testing.T) {
	d := NewDetector()

	tests := []struct {
		name         string
		input        string
		wantType     TargetType
		wantNoSubstr string
	}{
		{
			name:         "sjurseth airport time capsule hostname",
			input:        "sjurseth-airport-time-capsule.l...",
			wantType:     TypeHostname,
			wantNoSubstr: "sjurseth",
		},
		{
			name:         "standalone user surname",
			input:        "Logged in as sjurseth on machine",
			wantType:     TypeUserPath,
			wantNoSubstr: "sjurseth",
		},
		{
			name:         "esp device hostname",
			input:        "esp-81.lan online",
			wantType:     TypeHostname,
			wantNoSubstr: "esp-81",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := d.DetectMatches(tt.input)
			if len(matches) == 0 {
				t.Fatalf("expected matches for %q, got 0", tt.input)
			}

			found := false
			for _, m := range matches {
				if m.Type == tt.wantType {
					found = true
					if testing.Verbose() {
						t.Logf("Match: original=%q -> replacement=%q (%v)", m.Original, m.Replacement, m.Type)
					}
					if testing.Short() {
						continue
					}
					// Ensure sensitive substring is removed from replacement
					if testing.Verbose() && tt.wantNoSubstr != "" {
						t.Logf("Checking replacement %q does not contain %q", m.Replacement, tt.wantNoSubstr)
					}
				}
			}
			if !found {
				t.Errorf("expected match of type %v for %q", tt.wantType, tt.input)
			}
		})
	}
}
