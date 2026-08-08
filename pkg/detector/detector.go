package detector

import (
	"crypto/sha256"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// TargetType defines the type of sensitive data detected.
type TargetType string

const (
	TypeIPv4     TargetType = "ipv4"
	TypeIPv6     TargetType = "ipv6"
	TypeMAC      TargetType = "mac"
	TypeHostname TargetType = "hostname"
	TypeUserPath TargetType = "userpath"
	TypeEmail    TargetType = "email"
	TypeToken    TargetType = "token"
)

// Match represents a detected sensitive string and its replacement.
type Match struct {
	Original    string
	Replacement string
	Type        TargetType
	Start       int
	End         int
}

// Detector scans text or metadata for sensitive PII patterns.
type Detector struct {
	mapping      map[string]string
	targetSubnet string
}

// NewDetector initializes a detector instance with session-level mapping consistency.
func NewDetector() *Detector {
	return &Detector{
		mapping: make(map[string]string),
	}
}

var (
	ipv4Regex     = regexp.MustCompile(`\b(?:10|172\.(?:1[6-9]|2[0-9]|3[01])|192\.168)\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})\b`)
	macRegex      = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{2}[:-]){5}(?:[0-9A-Fa-f]{2})\b`)
	hostnameRegex = regexp.MustCompile(`\b[a-zA-Z0-9_-]+\.(?:lan|local|internal|home|hole)\b`)
	userPathRegex = regexp.MustCompile(`/Users/([a-zA-Z0-9_-]+)`)
	emailRegex    = regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`)
	tokenRegex    = regexp.MustCompile(`\b(?:ghp_|gho_|sk-|akip_)[a-zA-Z0-9_-]{20,}\b`)
)

// DetectMatches finds all sensitive strings in the input text and assigns consistent synthetic replacements.
func (d *Detector) DetectMatches(input string) []Match {
	var matches []Match

	// Detect IPv4 addresses
	for _, loc := range ipv4Regex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeIPv4(orig)
		matches = append(matches, Match{
			Original:    orig,
			Replacement: repl,
			Type:        TypeIPv4,
			Start:       loc[0],
			End:         loc[1],
		})
	}

	// Detect MAC addresses
	for _, loc := range macRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeMAC(orig)
		matches = append(matches, Match{
			Original:    orig,
			Replacement: repl,
			Type:        TypeMAC,
			Start:       loc[0],
			End:         loc[1],
		})
	}

	// Detect Hostnames
	for _, loc := range hostnameRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeHostname(orig)
		matches = append(matches, Match{
			Original:    orig,
			Replacement: repl,
			Type:        TypeHostname,
			Start:       loc[0],
			End:         loc[1],
		})
	}

	// Detect User Paths
	for _, loc := range userPathRegex.FindAllStringSubmatchIndex(input, -1) {
		if len(loc) >= 4 {
			user := input[loc[2]:loc[3]]
			replUser := d.anonymizeUser(user)
			orig := input[loc[0]:loc[1]]
			repl := strings.Replace(orig, user, replUser, 1)
			matches = append(matches, Match{
				Original:    orig,
				Replacement: repl,
				Type:        TypeUserPath,
				Start:       loc[0],
				End:         loc[1],
			})
		}
	}

	// Detect Emails
	for _, loc := range emailRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeEmail(orig)
		matches = append(matches, Match{
			Original:    orig,
			Replacement: repl,
			Type:        TypeEmail,
			Start:       loc[0],
			End:         loc[1],
		})
	}

	// Detect Tokens
	for _, loc := range tokenRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeToken(orig)
		matches = append(matches, Match{
			Original:    orig,
			Replacement: repl,
			Type:        TypeToken,
			Start:       loc[0],
			End:         loc[1],
		})
	}

	return matches
}

func (d *Detector) anonymizeIPv4(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}

	ip := net.ParseIP(orig)
	if ip == nil {
		return "10.0.1.10"
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "10.0.1.10"
	}

	// Guarantee consistent subnet prefix for all IPs in the same subnet
	subnetKey := fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2])
	if d.targetSubnet == "" {
		hash := sha256.Sum256([]byte(subnetKey))
		d.targetSubnet = fmt.Sprintf("10.0.%d", hash[0]%200+1)
	}

	repl := fmt.Sprintf("%s.%d", d.targetSubnet, ip4[3])
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeMAC(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	hash := sha256.Sum256([]byte(orig))
	repl := fmt.Sprintf("52:54:00:%02X:%02X:%02X", hash[0], hash[1], hash[2])
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeHostname(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}

	parts := strings.Split(orig, ".")
	name := parts[0]
	suffix := "internal"
	if len(parts) > 1 {
		suffix = strings.Join(parts[1:], ".")
	}

	// Preserve familiar hardware prefix (e.g. esp32, retropie, ha) while sanitizing user names
	var prefix string
	if strings.HasPrefix(name, "esp") {
		prefix = "esp-"
	} else if strings.HasPrefix(name, "ha") || strings.HasPrefix(name, "home-assistant") {
		prefix = "ha-"
	} else if strings.HasPrefix(name, "retropie") {
		prefix = "retro-"
	} else {
		prefix = "node-"
	}

	hash := sha256.Sum256([]byte(orig))
	repl := fmt.Sprintf("%s%02x.%s", prefix, hash[0], suffix)
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeUser(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	repl := "developer"
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeEmail(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	repl := "user@example.com"
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeToken(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	prefix := "token_"
	if strings.HasPrefix(orig, "gho_") {
		prefix = "gho_"
	} else if strings.HasPrefix(orig, "sk-") {
		prefix = "sk-"
	}
	repl := prefix + strings.Repeat("x", len(orig)-len(prefix))
	d.mapping[orig] = repl
	return repl
}
