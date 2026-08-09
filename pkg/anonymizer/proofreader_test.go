package anonymizer

import (
	"testing"

	"github.com/msjurset/anonymark/pkg/renderer"
)

func TestProofreaderAuditLayout(t *testing.T) {
	p := NewProofreader()
	targets := []renderer.TargetItem{
		{Original: "node-34.lan", Replacement: "node-56.lan", Type: "hostname"},
		{Original: "10.0.109.185", Replacement: "10.0.104.128", Type: "ipv4"},
	}

	report := p.AuditLayout(targets)
	if !report.Passed {
		t.Errorf("Expected AuditLayout to pass, got failed: %v", report.Feedback)
	}
}

func TestProofreaderAuditOutput(t *testing.T) {
	p := NewProofreader()
	fontSizes := []float64{16.0, 16.0, 16.0, 16.0}
	xMargins := []float64{100.0, 100.0, 100.0, 100.0}

	report := p.AuditOutput(fontSizes, xMargins)
	if !report.Passed {
		t.Errorf("Expected AuditOutput to pass for uniform font sizes, got: %v", report.Feedback)
	}
	if report.FontVariancePt != 0.0 {
		t.Errorf("Expected 0.0 font variance, got %f", report.FontVariancePt)
	}
}
