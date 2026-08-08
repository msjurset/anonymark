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

	// Subnet prefix (first 3 octets) should match
	sub1 := ip1[:len(ip1)-3]
	sub2 := ip2[:len(ip2)-3]

	if sub1 != sub2 {
		t.Errorf("subnet prefixes differ: %q vs %q", sub1, sub2)
	}
}
