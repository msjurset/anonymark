package detector

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/user"
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
	userTokens   []string
}

// NewDetector initializes a detector instance with session-level mapping consistency.
func NewDetector() *Detector {
	d := &Detector{
		mapping: make(map[string]string),
	}
	d.initUserTokens()
	return d
}

func (d *Detector) initUserTokens() {
	tokens := make(map[string]bool)

	// Add current OS user if available
	if u, err := user.Current(); err == nil && u.Username != "" {
		tokens[strings.ToLower(u.Username)] = true
	}
	if envUser := os.Getenv("USER"); envUser != "" {
		tokens[strings.ToLower(envUser)] = true
	}

	// Always protect common surname / username variations in environment if present
	for token := range tokens {
		if len(token) >= 3 {
			d.userTokens = append(d.userTokens, token)
			// If username is e.g. "msjurseth", also extract base surname "sjurseth"
			if strings.HasPrefix(token, "m") && len(token) > 4 {
				d.userTokens = append(d.userTokens, token[1:])
			}
		}
	}
	if len(d.userTokens) == 0 {
		d.userTokens = []string{"sjurseth", "msjurseth"}
	}
}

var (
	ipv4Regex       = regexp.MustCompile(`\b(?:10|172\.(?:1[6-9]|2[0-9]|3[01])|192\.168)\.(?:[0-9]{1,3})\.(?:[0-9]{1,3})\b`)
	macRegex        = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{2}[:-]){5}(?:[0-9A-Fa-f]{2})\b`)
	hostnameRegex   = regexp.MustCompile(`(?i)\b[a-zA-Z0-9_-]+\.(?:lan|local|internal|home|hole|domain|net|arpa|l\.\.\.|\.\.\.|\.l[a-z]*)\b`)
	deviceHostRegex = regexp.MustCompile(`(?i)\b(?:esp32|esp|ha|home-assistant|retropie|retro|node|pi-[a-zA-Z0-9_-]+|pi[0-9]+|airport|time-capsule|[a-zA-Z0-9_-]+-(?:airport|capsule|macbook|iphone|ipad|pc|nas|router|switch|ap|node|box|device|server|voice))[a-zA-Z0-9._-]*\b`)
	userPathRegex   = regexp.MustCompile(`/Users/([a-zA-Z0-9_-]+)`)
	emailRegex      = regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`)
	tokenRegex      = regexp.MustCompile(`\b(?:ghp_|gho_|sk-|akip_)[a-zA-Z0-9_-]{20,}\b`)
)

// DetectMatches finds all sensitive strings in the input text and assigns consistent synthetic replacements.
func (d *Detector) DetectMatches(input string) []Match {
	var matches []Match
	seen := make(map[string]bool)

	addMatch := func(orig, repl string, tType TargetType, start, end int) {
		key := fmt.Sprintf("%d:%d:%s", start, end, orig)
		if !seen[key] {
			seen[key] = true
			matches = append(matches, Match{
				Original:    orig,
				Replacement: repl,
				Type:        tType,
				Start:       start,
				End:         end,
			})
		}
	}

	// 1. Detect IPv4 addresses
	for _, loc := range ipv4Regex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeIPv4(orig)
		addMatch(orig, repl, TypeIPv4, loc[0], loc[1])
	}

	// 2. Detect MAC addresses
	for _, loc := range macRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeMAC(orig)
		addMatch(orig, repl, TypeMAC, loc[0], loc[1])
	}

	// 3. Detect Hostnames (standard domain extensions + device host patterns)
	for _, loc := range hostnameRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeHostname(orig)
		addMatch(orig, repl, TypeHostname, loc[0], loc[1])
	}
	for _, loc := range deviceHostRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeHostname(orig)
		addMatch(orig, repl, TypeHostname, loc[0], loc[1])
	}

	// 4. Detect User Paths & User Tokens anywhere in text
	for _, loc := range userPathRegex.FindAllStringSubmatchIndex(input, -1) {
		if len(loc) >= 4 {
			user := input[loc[2]:loc[3]]
			replUser := d.anonymizeUser(user)
			orig := input[loc[0]:loc[1]]
			repl := strings.Replace(orig, user, replUser, 1)
			addMatch(orig, repl, TypeUserPath, loc[0], loc[1])
		}
	}

	// Check for direct occurrences of user surname / username tokens
	inputLower := strings.ToLower(input)
	for _, token := range d.userTokens {
		pos := 0
		for {
			idx := strings.Index(inputLower[pos:], token)
			if idx == -1 {
				break
			}
			absStart := pos + idx
			absEnd := absStart + len(token)
			orig := input[absStart:absEnd]

			// Check if part of a hostname or standalone word
			wordStart := absStart
			for wordStart > 0 && isWordChar(input[wordStart-1]) {
				wordStart--
			}
			wordEnd := absEnd
			for wordEnd < len(input) && isWordChar(input[wordEnd]) {
				wordEnd++
			}
			fullWord := input[wordStart:wordEnd]

			if strings.Contains(fullWord, ".") || strings.Contains(fullWord, "-") {
				repl := d.anonymizeHostname(fullWord)
				addMatch(fullWord, repl, TypeHostname, wordStart, wordEnd)
			} else {
				repl := d.anonymizeUser(orig)
				addMatch(orig, repl, TypeUserPath, absStart, absEnd)
			}

			pos = absEnd
		}
	}

	// 5. Detect Emails
	for _, loc := range emailRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeEmail(orig)
		addMatch(orig, repl, TypeEmail, loc[0], loc[1])
	}

	// 6. Detect Tokens
	for _, loc := range tokenRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeToken(orig)
		addMatch(orig, repl, TypeToken, loc[0], loc[1])
	}

	return matches
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-' || b == '.'
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
	suffix := "lan"
	if len(parts) > 1 {
		s := strings.TrimRight(parts[1], ".")
		if s == "l" || s == "local" || strings.HasPrefix(s, "loc") {
			suffix = "local"
		} else if s != "" && !strings.HasPrefix(s, "l..") && !strings.HasPrefix(s, "l.") {
			suffix = s
		}
	}

	var prefix string
	lowerName := strings.ToLower(name)

	if lowerName == "pi" {
		prefix = "n-"
	} else if strings.HasPrefix(lowerName, "esp") {
		prefix = "esp-"
	} else if strings.HasPrefix(lowerName, "ha") || strings.HasPrefix(lowerName, "home-assistant") {
		prefix = "ha-"
	} else if strings.HasPrefix(lowerName, "retropie") || strings.HasPrefix(lowerName, "retro") {
		prefix = "retro-"
	} else if strings.Contains(lowerName, "capsule") || strings.Contains(lowerName, "airport") {
		prefix = "node-capsule-"
	} else {
		prefix = "node-"
	}

	hash := sha256.Sum256([]byte(orig))
	hashNum := int(hash[0]) % 90 + 10
	repl := fmt.Sprintf("%s%d.%s", prefix, hashNum, suffix)
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
