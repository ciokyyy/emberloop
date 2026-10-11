// Heuristic miner patterns ported from claude-reflect (MIT © 2025 Bayram
// Annakov, https://github.com/BayramAnnakov/claude-reflect,
// scripts/lib/reflect_utils.py). The notice is kept in NOTICE.md beside
// schema.go. The port keeps the table contents, the fixed evaluation order
// (slash-command guard, length floor, explicit, guardrail, false-positive,
// non-correction, positive with forward-pivot guard, CJK, corrections with
// weak-length gates) and the confidence adjustments.
//
// The Daemon miner reads events.jsonl only through FilterMinerEvents (see
// emberloop-web/injection.go); ClassifyHeuristic only judges one text.
package memory

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Heuristic types returned by ClassifyHeuristic.
const (
	HitExplicit  = "explicit"
	HitGuardrail = "guardrail"
	HitPositive  = "positive"
	HitAuto      = "auto"
)

// HeuristicResult is one heuristic hit: what fired and how confident it is.
type HeuristicResult struct {
	Type       string
	Patterns   string
	Confidence float64
	Sentiment  string
	DecayDays  int
}

type explicitPat struct {
	re         *regexp.Regexp
	name       string
	confidence float64
	decay      int
}

type correctionPat struct {
	re     *regexp.Regexp
	name   string
	strong bool
}

type cjkPat struct {
	re     *regexp.Regexp
	name   string
	strong bool
}

var (
	slashCmdRE = regexp.MustCompile(`^/[A-Za-z][0-9A-Za-z_.\-]*(?::[0-9A-Za-z_.\-]+)?(?:\s|$)`)

	explicitPatterns = []explicitPat{
		{regexp.MustCompile(`(?i)remember:`), "remember:", 0.90, 120},
	}

	guardrailPatterns = []explicitPat{
		{regexp.MustCompile(`(?i)don't (?:add|include|create) .{1,40} unless`), "dont-unless-asked", 0.90, 120},
		{regexp.MustCompile(`(?i)only (?:change|modify|edit|touch) what I (?:asked|requested|said)`), "only-what-asked", 0.90, 120},
		{regexp.MustCompile(`(?i)stop (?:refactoring|changing|modifying|editing) (?:unrelated|other|surrounding)`), "stop-unrelated", 0.90, 120},
		{regexp.MustCompile(`(?i)don't (?:over-engineer|add extra|be too|make unnecessary)`), "dont-over-engineer", 0.85, 90},
		{regexp.MustCompile(`(?i)don't (?:refactor|reorganize|restructure) (?:unless|without)`), "dont-refactor-unless", 0.85, 90},
		{regexp.MustCompile(`(?i)leave .{1,30} (?:alone|unchanged|as is)`), "leave-alone", 0.85, 90},
		{regexp.MustCompile(`(?i)don't (?:add|include) (?:comments|docstrings|type hints|annotations) (?:unless|to code)`), "dont-add-annotations", 0.85, 90},
		{regexp.MustCompile(`(?i)(?:minimal|minimum|only necessary) changes`), "minimal-changes", 0.80, 90},
	}

	falsePositivePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)[?` + "\uFF1F" + `]$`),
		regexp.MustCompile("[\u55CE\u5417\u5462\u304B\uAE4C]$"),
		regexp.MustCompile(`(?i)^(please|can you|could you|would you|help me)\b`),
		regexp.MustCompile(`(?i)(help|fix|check|review|figure out|set up)\s+(this|that|it|the)\b`),
		regexp.MustCompile(`(?i)(error|failed|could not|cannot|can't|unable to)\s+\w+`),
		regexp.MustCompile(`(?i)(is|was|are|were)\s+(not|broken|failing)`),
		regexp.MustCompile(`(?i)^I (need|want|would like)\b`),
		regexp.MustCompile(`(?i)^(ok|okay|alright)[,.]?\s+(so|now|let)`),
	}

	nonCorrectionPhrases = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^no\s+problem`),
		regexp.MustCompile(`(?i)^no\s+worries`),
		regexp.MustCompile(`(?i)^no\s+need\b`),
		regexp.MustCompile(`(?i)^no\s+way\b`),
		regexp.MustCompile(`(?i)^don't\s+worry`),
		regexp.MustCompile(`(?i)^don't\s+mind`),
		regexp.MustCompile(`(?i)^don't\s+bother`),
		regexp.MustCompile(`(?i)^never\s+mind`),
		regexp.MustCompile(`(?i)^no\s+idea\b`),
		regexp.MustCompile(`(?i)^no\s+(?:it|that|this|we|i|you|they)\s+(?:works?|worked|looks?|seems?|sounds?|reads?)\b`),
		regexp.MustCompile(`(?i)^no\s+(?:it|that|this)\s+(?:'s|is|was)\s+(?:fine|good|ok|okay|right|correct)\b`),
		regexp.MustCompile(`(?i)^no\s+(?:i|we)\s+(?:think|guess|believe|reckon)\b`),
		regexp.MustCompile(`(?i)^no\s+(?:you|we|i)\s+(?:can|could|should)\s+go\s+ahead\b`),
		regexp.MustCompile(`(?i)^no\s+(?:i|we)(?:'m|'re| am| are)?\s+(?:all\s+)?(?:good|done|set|fine)\b`),
		regexp.MustCompile(`(?i)^no\s+[\w\-]+\s+(?:appeared|happened|occurred|showed|showed\s+up|returned|existed|came|come|changed|matched|was|were|has|have|had)\b`),
		regexp.MustCompile(`(?i)^no\s+(?:rush|hurry|pressure|problem|stress)\b`),
		regexp.MustCompile(`(?i)^stop\s+worrying`),
	}

	positivePatterns = []explicitPat{
		{regexp.MustCompile(`(?i)perfect!|exactly right|that's exactly`), "perfect", 0.70, 90},
		{regexp.MustCompile(`(?i)that's what I wanted|great approach`), "great-approach", 0.70, 90},
		{regexp.MustCompile(`(?i)keep doing this|love it|excellent|nailed it`), "keep-doing", 0.70, 90},
	}

	forwardPivotPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(now|next)[, ]+let'?s\b`),
		regexp.MustCompile(`(?i)\b(now|next)[, ]+(we|i)\s+(need to|should|have to|must|will)\b`),
		regexp.MustCompile(`(?i)\blet'?s (add|do|build|move|update|change|fix|implement)\b`),
		regexp.MustCompile(`(?i)\bgo ahead and\s+\w+`),
	}

	correctionPatterns = []correctionPat{
		{regexp.MustCompile("(?i)^no[,\\.!:;—–\\-]+\\s*\\S"), "no,", true},
		{regexp.MustCompile(`(?i)^no\s+\S`), "no-bare", true},
		{regexp.MustCompile(`(?i)^don't\b|^do not\b`), "don't", true},
		{regexp.MustCompile(`(?i)^stop\b|^never\b`), "stop/never", true},
		{regexp.MustCompile(`(?i)that's (wrong|incorrect)|that is (wrong|incorrect)`), "that's-wrong", true},
		{regexp.MustCompile(`(?i)^actually[,. ]`), "actually", false},
		{regexp.MustCompile(`(?i)^I meant\b|^I said\b`), "I-meant/said", true},
		{regexp.MustCompile(`(?i)^I told you\b|^I already told\b`), "I-told-you", true},
		{regexp.MustCompile(`(?i)use .{1,30} not\b`), "use-X-not-Y", true},
	}

	cjkCorrectionPatterns = []cjkPat{
		{regexp.MustCompile(`^いや[、,.\s]|^いや違`), "iya", true},
		{regexp.MustCompile(`^違う[、，,.\s！!。]|^ちがう[、,.\s]`), "chigau", true},
		{regexp.MustCompile(`そうじゃなく[てけ]|そっちじゃなく[てけ]`), "souja-nakute", true},
		{regexp.MustCompile(`間違[いえっ]て`), "machigatte", true},
		{regexp.MustCompile(`じゃなくて.{0,30}にして`), "janakute-nishite", true},
		{regexp.MustCompile(`^やめて[。！!]?[\s]*$`), "yamete", true},
		{regexp.MustCompile(`^そうじゃない`), "souja-nai", true},
		{regexp.MustCompile(`って言った[のよでじゃ]`), "tte-itta", true},
		{regexp.MustCompile(`^不是[，,. ]`), "bushi", true},
		{regexp.MustCompile(`^错了|^錯了`), "cuole", true},
		{regexp.MustCompile(`不要.{0,20}要`), "buyao-yao", true},
		{regexp.MustCompile(`^아니[,. ]`), "ani", true},
		{regexp.MustCompile(`틀렸`), "teullyeoss", true},
	}
)

// Miner thresholds ported from claude-reflect.
const (
	maxWeakPatternLength   = 150
	minShortCorrectionLen  = 80
	minPositiveContextLen  = 25
	maxCapturePromptLength = 500
)

// hasCJK reports whether s holds a CJK character, which carries more meaning
// per char, so the length floor drops for it.
func hasCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x3000 && r <= 0x9FFF,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0xAC00 && r <= 0xD7AF:
			return true
		}
	}
	return false
}

// ClassifyHeuristic judges one user text in the fixed upstream order. It
// reports the hit and true, or false when the text holds nothing durable.
// Slash commands, too-short texts, questions, requests, denylisted phrases,
// bare praise and forward pivots are all rejected before a hit.
func ClassifyHeuristic(text string) (HeuristicResult, bool) {
	// Slash-command guard: skill invocations are never user feedback, even
	// when their expanded bodies hold correction tokens.
	if slashCmdRE.MatchString(strings.TrimLeft(text, " \t\r\n")) {
		return HeuristicResult{}, false
	}
	stripped := strings.TrimSpace(text)
	shortThreshold := 4
	if hasCJK(stripped) {
		shortThreshold = 2
	}
	if utf8.RuneCountInString(stripped) <= shortThreshold {
		return HeuristicResult{}, false
	}
	for _, p := range explicitPatterns {
		if p.re.MatchString(text) {
			return HeuristicResult{Type: HitExplicit, Patterns: p.name, Confidence: p.confidence, Sentiment: "correction", DecayDays: p.decay}, true
		}
	}
	for _, p := range guardrailPatterns {
		if p.re.MatchString(text) {
			return HeuristicResult{Type: HitGuardrail, Patterns: p.name, Confidence: p.confidence, Sentiment: "correction", DecayDays: p.decay}, true
		}
	}
	for _, re := range falsePositivePatterns {
		if re.MatchString(text) {
			return HeuristicResult{}, false
		}
	}
	for _, re := range nonCorrectionPhrases {
		if re.MatchString(text) {
			return HeuristicResult{}, false
		}
	}
	var matchedPositive []string
	var positiveDecay = 90
	for _, p := range positivePatterns {
		if p.re.MatchString(text) {
			matchedPositive = append(matchedPositive, p.name)
			positiveDecay = p.decay
		}
	}
	if len(matchedPositive) > 0 {
		if utf8.RuneCountInString(strings.TrimSpace(text)) < minPositiveContextLen {
			return HeuristicResult{}, false
		}
		for _, re := range forwardPivotPatterns {
			if re.MatchString(text) {
				return HeuristicResult{}, false
			}
		}
		return HeuristicResult{Type: HitPositive, Patterns: strings.Join(matchedPositive, " "), Confidence: 0.70, Sentiment: "positive", DecayDays: positiveDecay}, true
	}
	textLen := len(text)
	var matchedCJK []string
	cjkStrong := false
	for _, p := range cjkCorrectionPatterns {
		if p.re.MatchString(stripped) {
			matchedCJK = append(matchedCJK, p.name)
			if p.strong {
				cjkStrong = true
			}
		}
	}
	if len(matchedCJK) > 0 {
		confidence := 0.60
		decay := 60
		if cjkStrong {
			confidence, decay = 0.75, 90
		}
		if textLen < minShortCorrectionLen {
			confidence = min(confidence+0.10, 0.90)
		} else if textLen > 300 {
			confidence = max(confidence-0.15, 0.50)
		}
		return HeuristicResult{Type: HitAuto, Patterns: strings.Join(matchedCJK, " "), Confidence: confidence, Sentiment: "correction", DecayDays: decay}, true
	}
	var matched []string
	count := 0
	strong := false
	toldYou := false
	for _, p := range correctionPatterns {
		if !p.re.MatchString(text) {
			continue
		}
		if !p.strong && textLen > maxWeakPatternLength {
			continue
		}
		matched = append(matched, p.name)
		count++
		if p.strong {
			strong = true
		}
		if p.name == "I-told-you" {
			toldYou = true
		}
	}
	if len(matched) > 0 {
		var confidence float64
		var decay int
		switch {
		case toldYou:
			confidence, decay = 0.85, 120
		case count >= 3:
			confidence, decay = 0.85, 120
		case count >= 2:
			confidence, decay = 0.75, 90
		case strong:
			confidence, decay = 0.70, 60
		default:
			confidence, decay = 0.55, 45
		}
		switch {
		case textLen < minShortCorrectionLen:
			confidence = min(confidence+0.10, 0.90)
		case textLen > 300:
			confidence = max(confidence-0.15, 0.50)
		case textLen > 150:
			confidence = max(confidence-0.10, 0.55)
		}
		return HeuristicResult{Type: HitAuto, Patterns: strings.Join(matched, " "), Confidence: confidence, Sentiment: "correction", DecayDays: decay}, true
	}
	return HeuristicResult{}, false
}
