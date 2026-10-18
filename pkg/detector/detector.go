package detector

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/user"
	"regexp"
	"sort"
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

	if u, err := user.Current(); err == nil && u.Username != "" {
		tokens[strings.ToLower(u.Username)] = true
	}
	if envUser := os.Getenv("USER"); envUser != "" {
		tokens[strings.ToLower(envUser)] = true
	}

	for token := range tokens {
		if len(token) >= 3 {
			d.userTokens = append(d.userTokens, token)
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
	macRegex        = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{1,2}[:-]){5}(?:[0-9A-Fa-f]{1,2})\b`)
	hostnameRegex   = regexp.MustCompile(`(?i)\b[a-zA-Z0-9][a-zA-Z0-9_-]*\.(?:lan|local|internal|home|hole|domain|net|arpa|l\.\.\.|lo\.\.\.|loc\.\.\.|\.\.\.)\b`)
	deviceHostRegex = regexp.MustCompile(`(?i)\b(?:esp32|esp|ha|home-assistant|retropie|retro|node|pi-[a-zA-Z0-9_-]+|pi[0-9]+|porkchop[a-zA-Z0-9_-]*|airport|time-capsule|[a-zA-Z0-9_-]+-(?:airport|capsule|macbook|iphone|ipad|pc|nas|router|switch|ap|node|box|device|server|voice))[a-zA-Z0-9._-]*`)
	userPathRegex   = regexp.MustCompile(`/Users/([a-zA-Z0-9_-]+)`)
	emailRegex      = regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b`)
	tokenRegex      = regexp.MustCompile(`\b(?:ghp_|gho_|sk-|akip_)[a-zA-Z0-9_-]{20,}\b`)
)

// DetectMatches finds all sensitive strings in the input text and assigns consistent synthetic replacements.
// Overlapping span matches (e.g. a device prefix inside a full .lan hostname) are deduplicated so only the
// longest complete span at any position is returned.
func (d *Detector) DetectMatches(input string) []Match {
	var candidates []Match

	addCandidate := func(orig, repl string, tType TargetType, start, end int) {
		if orig == "" || start < 0 || end <= start {
			return
		}
		candidates = append(candidates, Match{
			Original:    orig,
			Replacement: repl,
			Type:        tType,
			Start:       start,
			End:         end,
		})
	}

	// 1. Detect Emails first (so domain part isn't mistaken for a hostname)
	for _, loc := range emailRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeEmail(orig)
		addCandidate(orig, repl, TypeEmail, loc[0], loc[1])
	}

	// 2. Detect Tokens
	for _, loc := range tokenRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeToken(orig)
		addCandidate(orig, repl, TypeToken, loc[0], loc[1])
	}

	// 3. Detect IPv4 addresses
	for _, loc := range ipv4Regex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeIPv4(orig)
		addCandidate(orig, repl, TypeIPv4, loc[0], loc[1])
	}

	// 4. Detect MAC addresses
	for _, loc := range macRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeMAC(orig)
		addCandidate(orig, repl, TypeMAC, loc[0], loc[1])
	}

	// 5. Detect Hostnames (standard domain extensions + device host patterns)
	for _, loc := range hostnameRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeHostname(orig)
		addCandidate(orig, repl, TypeHostname, loc[0], loc[1])
	}
	for _, loc := range deviceHostRegex.FindAllStringIndex(input, -1) {
		orig := input[loc[0]:loc[1]]
		repl := d.anonymizeHostname(orig)
		addCandidate(orig, repl, TypeHostname, loc[0], loc[1])
	}

	// 6. Detect User Paths & User Tokens anywhere in text
	for _, loc := range userPathRegex.FindAllStringSubmatchIndex(input, -1) {
		if len(loc) >= 4 {
			uName := input[loc[2]:loc[3]]
			replUser := d.anonymizeUser(uName)
			orig := input[loc[0]:loc[1]]
			repl := strings.Replace(orig, uName, replUser, 1)
			addCandidate(orig, repl, TypeUserPath, loc[0], loc[1])
		}
	}

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
				addCandidate(fullWord, repl, TypeHostname, wordStart, wordEnd)
			} else {
				repl := d.anonymizeUser(orig)
				addCandidate(orig, repl, TypeUserPath, absStart, absEnd)
			}

			pos = absEnd
		}
	}

	// Sort candidates: longest span first, then earliest start
	sort.SliceStable(candidates, func(i, j int) bool {
		lenI := candidates[i].End - candidates[i].Start
		lenJ := candidates[j].End - candidates[j].Start
		if lenI != lenJ {
			return lenI > lenJ
		}
		return candidates[i].Start < candidates[j].Start
	})

	// Accept non-overlapping spans
	var accepted []Match
	for _, c := range candidates {
		overlaps := false
		for _, a := range accepted {
			if c.Start < a.End && c.End > a.Start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			accepted = append(accepted, c)
		}
	}

	// Sort accepted matches in reading order (by Start ascending)
	sort.Slice(accepted, func(i, j int) bool {
		return accepted[i].Start < accepted[j].Start
	})

	return accepted
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
		return "10.0.20.10"
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "10.0.20.10"
	}

	subnetKey := fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2])
	if d.targetSubnet == "" {
		hash := sha256.Sum256([]byte(subnetKey))
		d.targetSubnet = fmt.Sprintf("10.0.%d", hash[0]%180+20)
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

	hasEllipsis := strings.HasSuffix(orig, "...")
	cleaned := strings.TrimSuffix(orig, "...")
	parts := strings.Split(cleaned, ".")
	name := parts[0]
	suffix := "lan"
	if len(parts) > 1 {
		s := strings.ToLower(strings.TrimRight(parts[len(parts)-1], "."))
		switch {
		case s == "local" || s == "l" || strings.HasPrefix(s, "loc"):
			suffix = "local"
		case s == "hole":
			suffix = "lan"
		case s != "":
			suffix = s
		}
	}

	hash := sha256.Sum256([]byte(orig))
	hashNum := int(hash[0])%80 + 11
	lowerName := strings.ToLower(name)

	var base string
	switch {
	case lowerName == "pi" || strings.Contains(lowerName, "pihole") || strings.HasSuffix(strings.ToLower(orig), ".hole"):
		base = fmt.Sprintf("dns-sink-%02d", hashNum)
	case strings.HasPrefix(lowerName, "esp"):
		base = fmt.Sprintf("iot-node-%02d", hashNum)
	case strings.HasPrefix(lowerName, "ha") || strings.HasPrefix(lowerName, "home-assistant"):
		base = fmt.Sprintf("automation-%02d", hashNum)
	case strings.HasPrefix(lowerName, "retro"):
		base = fmt.Sprintf("media-box-%02d", hashNum)
	case strings.Contains(lowerName, "capsule") || strings.Contains(lowerName, "airport"):
		base = fmt.Sprintf("backup-nas-%02d", hashNum)
	default:
		roles := []string{"core-node", "lab-host", "edge-gw", "storage-nas", "compute-srv", " worker-node", "proxy-gw", "metrics-db"}
		role := strings.TrimSpace(roles[int(hash[1])%len(roles)])
		base = fmt.Sprintf("%s-%02d", role, hashNum)
	}

	// Expand with whole, natural hyphenated tokens (never slicing a token mid-word) if original was much longer
	wholeMods := []string{"-primary", "-cluster", "-rack", "-gateway", "-storage"}
	candidate := fmt.Sprintf("%s.%s", base, suffix)
	modIdx := int(hash[2])
	for len(candidate)+6 <= len(orig) && modIdx < int(hash[2])+len(wholeMods) {
		mod := wholeMods[modIdx%len(wholeMods)]
		modIdx++
		next := fmt.Sprintf("%s%s.%s", base, mod, suffix)
		if len(next) <= len(orig)+2 {
			base += mod
			candidate = next
		} else {
			break
		}
	}

	if hasEllipsis && len(orig) > 18 {
		candidate = fmt.Sprintf("%s...", base)
	}

	d.mapping[orig] = candidate
	return candidate
}

func (d *Detector) anonymizeUser(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	repl := "operator"
	d.mapping[orig] = repl
	return repl
}

func (d *Detector) anonymizeEmail(orig string) string {
	if repl, exists := d.mapping[orig]; exists {
		return repl
	}
	repl := "ops@example.com"
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
	repl := prefix + strings.Repeat("x", max(1, len(orig)-len(prefix)))
	d.mapping[orig] = repl
	return repl
}

func fitLength(s string, targetLen int) string {
	if len(s) == targetLen || targetLen <= 0 {
		return s
	}
	if len(s) > targetLen {
		if idx := strings.LastIndex(s, "."); idx > 0 && targetLen > len(s)-idx {
			name := s[:idx]
			ext := s[idx:]
			maxNameLen := targetLen - len(ext)
			if maxNameLen > 0 {
				return name[:maxNameLen] + ext
			}
		}
		return s[:targetLen]
	}

	idx := strings.LastIndex(s, ".")
	name := s
	ext := ""
	if idx > 0 {
		name = s[:idx]
		ext = s[idx:]
	}

	wordPool := []string{"-primary", "-gateway", "-router", "-storage", "-control", "-backup", "-server", "-switch", "-access", "-station"}
	buf := name
	wIdx := 0

	for len(buf)+len(ext) < targetLen {
		w := wordPool[wIdx%len(wordPool)]
		wIdx++
		needed := targetLen - len(buf) - len(ext)
		if len(w) <= needed {
			buf += w
		} else {
			break
		}
	}

	return buf + ext
}
