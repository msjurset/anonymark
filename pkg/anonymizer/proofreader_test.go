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

func TestProofreaderAuditRenderedRegions(t *testing.T) {
	p := NewProofreader()
	records := []RegionAuditRecord{
		{OriginalText: "node-34.lan", Replacement: "node-56.lan", TargetWidth: 100.0, RenderedWidth: 100.0, LineHeight: 26.0, FontSizePt: 20.0, WidthCoverage: 1.0},
		{OriginalText: "10.0.109.185", Replacement: "10.0.104.128", TargetWidth: 120.0, RenderedWidth: 119.5, LineHeight: 26.0, FontSizePt: 20.0, WidthCoverage: 0.995},
	}

	report := p.AuditRenderedRegions(records)
	if !report.Passed {
		t.Errorf("Expected AuditRenderedRegions to pass for uniform coverage, got: %v", report.Feedback)
	}
	if report.DefectCount != 0 {
		t.Errorf("Expected 0 defects, got %d", report.DefectCount)
	}
}

func TestProofreaderAuditRenderedRegionsDefect(t *testing.T) {
	p := NewProofreader()
	records := []RegionAuditRecord{
		{OriginalText: "node-34.lan", Replacement: "node-56.lan", TargetWidth: 150.0, RenderedWidth: 100.0, LineHeight: 26.0, FontSizePt: 15.0, WidthCoverage: 0.666},
	}

	report := p.AuditRenderedRegions(records)
	if report.Passed {
		t.Errorf("Expected AuditRenderedRegions to fail for width gap defect, but passed")
	}
	if report.DefectCount != 1 {
		t.Errorf("Expected 1 defect, got %d", report.DefectCount)
	}
}
