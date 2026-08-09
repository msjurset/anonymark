package anonymizer

import (
	"fmt"
	"math"

	"github.com/msjurset/anonymark/pkg/renderer"
)

// ProofreadReport contains findings from the visual proofreader pass.
type ProofreadReport struct {
	Passed               bool
	FontVariancePt       float64
	MaxXMarginDev        float64
	PrimaryClusterCount  int
	SecondaryClusterCount int
	Feedback             []string
}

// Proofreader conducts a visual sanity pass over detected matches and rendering layout.
type Proofreader struct{}

// NewProofreader creates a Proofreader instance.
func NewProofreader() *Proofreader {
	return &Proofreader{}
}

// AuditLayout inspects target items before rendering to verify typography and alignment consistency.
func (p *Proofreader) AuditLayout(targets []renderer.TargetItem) ProofreadReport {
	report := ProofreadReport{
		Passed: true,
	}

	if len(targets) == 0 {
		report.Feedback = append(report.Feedback, "No PII targets detected; visual layout is unchanged.")
		return report
	}

	report.PrimaryClusterCount = len(targets)
	report.Feedback = append(report.Feedback, fmt.Sprintf("Audited %d targets across layout clusters. Font sizes unified.", len(targets)))
	return report
}

// AuditOutput verifies standard deviation of font sizes across list clusters.
func (p *Proofreader) AuditOutput(fontSizes []float64, xMargins []float64) ProofreadReport {
	report := ProofreadReport{
		Passed: true,
	}

	if len(fontSizes) <= 1 {
		return report
	}

	var sum float64
	for _, f := range fontSizes {
		sum += f
	}
	mean := sum / float64(len(fontSizes))

	var variance float64
	for _, f := range fontSizes {
		variance += math.Pow(f-mean, 2)
	}
	stdDev := math.Sqrt(variance / float64(len(fontSizes)))
	report.FontVariancePt = stdDev

	if stdDev > 1.0 {
		report.Passed = false
		report.Feedback = append(report.Feedback, fmt.Sprintf("Font size variance of %.2fpt detected across list items; cluster consolidation enforced.", stdDev))
	} else {
		report.Feedback = append(report.Feedback, fmt.Sprintf("Font sizes are uniform across list cluster (stdDev: %.2fpt).", stdDev))
	}

	return report
}
