// Dedup relations for mined observations.
//
// Normalization lowercases, splits on non-letters and drops filler stopwords,
// but keeps every negation token and polarity adverb, so "always return
// wrapped errors" and "never return wrapped errors" stay distinct. Overlapping
// pairs get a relation: duplicate (same normalized text), contradicts (same
// substance, opposite polarity) or refines (one contains the other or they
// overlap heavily). Nothing is ever dropped: refines and contradicts are both
// stored, and only duplicates fold into evidence_count.
//
// The Daemon miner reads events.jsonl only through FilterMinerEvents (see
// emberloop-web/injection.go); these helpers only compare quotes.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

// Dedup relations between two observations.
const (
	RelDuplicate   = "duplicate"
	RelRefines     = "refines"
	RelContradicts = "contradicts"
)

// negationTokens are never dropped by normalization: polarity decides
// contradicts, so it must survive.
var negationTokens = map[string]bool{
	"no": true, "not": true, "never": true, "none": true, "neither": true,
	"nor": true, "nobody": true, "nothing": true, "nowhere": true,
	"don't": true, "doesn't": true, "didn't": true, "won't": true,
	"wouldn't": true, "shouldn't": true, "can't": true, "cannot": true,
	"couldn't": true, "mustn't": true, "needn't": true, "n't": true,
	"without": true, "unless": true, "hardly": true, "barely": true,
	"rarely": true, "seldom": true,
	// Polarity adverbs: dropping one would merge opposites.
	"always": true, "sometimes": true, "often": true,
	"usually": true,
}

// antonymPairs are opposite adverbs that mark contradicts when the rest of
// the quote overlaps.
var antonymPairs = [][2]string{
	{"always", "never"},
	{"always", "sometimes"},
	{"all", "none"},
	{"every", "never"},
	{"must", "mustn't"},
}

// fillerStopwords are dropped by normalization. Negation tokens and polarity
// adverbs are never in this set.
var fillerStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "to": true,
	"of": true, "in": true, "on": true, "at": true, "for": true, "with": true,
	"and": true, "or": true, "but": true, "if": true, "then": true,
	"else": true, "it": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "you": true, "he": true, "she": true,
	"we": true, "they": true, "my": true, "your": true, "his": true,
	"her": true, "our": true, "their": true, "me": true, "him": true,
	"us": true, "them": true, "as": true, "by": true, "from": true,
	"please": true, "just": true, "very": true, "really": true,
	"so": true, "such": true, "do": true, "does": true, "did": true,
	"have": true, "has": true, "had": true, "will": true, "would": true,
	"should": true, "could": true, "can": true, "may": true, "might": true,
	"about": true, "into": true, "over": true, "after": true, "before": true,
	"between": true, "through": true, "during": true, "each": true,
	"other": true, "some": true, "what": true, "which": true, "who": true,
	"when": true, "where": true, "how": true,
}

// tokenizeQuote lowercases s, splits on anything but letters, digits and
// apostrophes, and drops filler stopwords while keeping negation tokens.
func tokenizeQuote(s string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		w := cur.String()
		cur.Reset()
		if w == "" {
			return
		}
		if fillerStopwords[w] && !negationTokens[w] {
			return
		}
		toks = append(toks, w)
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return toks
}

// NormalizeQuote normalizes s for dedup: lowercase tokens, filler dropped,
// negation kept, joined with single spaces.
func NormalizeQuote(s string) string {
	return strings.Join(tokenizeQuote(s), " ")
}

// tokenSet returns the set of tokens in the normalized quote.
func tokenSet(norm string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(norm) {
		set[w] = true
	}
	return set
}

// jaccardSet is the Jaccard similarity of two token sets.
func jaccardSet(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

// containsAll reports whether every token of sub is in full.
func containsAll(full, sub map[string]bool) bool {
	for w := range sub {
		if !full[w] {
			return false
		}
	}
	return len(sub) > 0
}

// hasNegation reports whether the token set holds a negation token.
func hasNegation(set map[string]bool) bool {
	for w := range set {
		if negationTokens[w] {
			return true
		}
	}
	return false
}

// overlapWithout reports the Jaccard similarity after removing the named
// tokens from both sets.
func overlapWithout(a, b map[string]bool, drop ...string) float64 {
	strip := func(set map[string]bool) map[string]bool {
		out := make(map[string]bool, len(set))
		for w := range set {
			out[w] = true
		}
		for _, d := range drop {
			delete(out, d)
		}
		return out
	}
	return jaccardSet(strip(a), strip(b))
}

// negationsOf lists the negation tokens in the set.
func negationsOf(set map[string]bool) []string {
	var out []string
	for w := range set {
		if negationTokens[w] {
			out = append(out, w)
		}
	}
	return out
}

// isContradiction reports opposite polarity on overlapping substance: a
// negation difference with the non-negation tokens overlapping, or an
// antonym swap with the rest overlapping.
func isContradiction(a, b map[string]bool) bool {
	if jaccardSet(a, b) < 0.4 {
		return false
	}
	if hasNegation(a) != hasNegation(b) {
		drop := append(negationsOf(a), negationsOf(b)...)
		if overlapWithout(a, b, drop...) >= 0.6 {
			return true
		}
	}
	for _, p := range antonymPairs {
		if (a[p[0]] && b[p[1]]) || (a[p[1]] && b[p[0]]) {
			if overlapWithout(a, b, p[0], p[1]) >= 0.5 {
				return true
			}
		}
	}
	return false
}

// Relation assigns the dedup relation of two quotes: duplicate when the
// normalized texts match, contradicts when the substance overlaps with
// opposite polarity, refines when one contains the other or they overlap
// heavily, else empty. Empty normalizations never relate.
func Relation(a, b string) string {
	na, nb := NormalizeQuote(a), NormalizeQuote(b)
	if na == "" || nb == "" {
		return ""
	}
	if na == nb {
		return RelDuplicate
	}
	sa, sb := tokenSet(na), tokenSet(nb)
	if isContradiction(sa, sb) {
		return RelContradicts
	}
	if containsAll(sa, sb) || containsAll(sb, sa) {
		return RelRefines
	}
	if jaccardSet(sa, sb) >= 0.5 {
		return RelRefines
	}
	return ""
}

// DedupAndStore stores obs in mem with dedup: a replay of the same Session
// Event returns its ID without bumping, a duplicate of a stored observation
// bumps that row's evidence_count and returns its ID, while refines,
// contradicts and novel quotes are all inserted and nothing is dropped. An
// inserted row carries the relation to its closest stored observation and
// that row's ID in its decider_json, so the later queue stage reads it
// without recomputing from quotes. It returns the stored ID and the
// relation (empty when novel).
func DedupAndStore(ctx context.Context, mem Memory, obs Observation) (int64, string, error) {
	hits, err := mem.SearchObservations(ctx, obs.Quote, 20)
	if err != nil {
		return 0, "", err
	}
	for _, h := range hits {
		if h.SessionID == obs.SessionID && h.EventID == obs.EventID {
			return h.ID, RelDuplicate, nil
		}
	}
	best, bestID := "", int64(0)
	for _, h := range hits {
		rel := Relation(obs.Quote, h.Quote)
		if rel == "" {
			continue
		}
		if rel == RelDuplicate {
			updated, err := mem.BumpObservationEvidence(ctx, h.ID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					break // the row vanished under us; store anew below.
				}
				return 0, "", err
			}
			return updated.ID, RelDuplicate, nil
		}
		if best == "" {
			best, bestID = rel, h.ID
		}
	}
	if best != "" {
		obs.DeciderJSON = withRelation(obs.DeciderJSON, best, bestID)
	}
	id, err := mem.AddObservation(ctx, obs)
	if err != nil {
		return 0, "", err
	}
	return id, best, nil
}

// withRelation records the dedup relation to a stored observation in a
// decider_json document, merging into the existing keys when it parses.
func withRelation(doc, rel string, relatedID int64) string {
	var m map[string]any
	if doc == "" || json.Unmarshal([]byte(doc), &m) != nil {
		m = map[string]any{}
	}
	m["relation"] = rel
	m["related_observation_id"] = relatedID
	raw, err := json.Marshal(m)
	if err != nil {
		return doc
	}
	return string(raw)
}
