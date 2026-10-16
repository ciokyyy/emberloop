// Secret-redaction patterns adapted from deja-vu (MIT © 2026 Vladislav
// Shulcz, https://github.com/vshulcz/deja-vu, internal/redact/redact.go).
// The notice is kept in NOTICE.md beside schema.go. This port keeps
// deja-vu's marker form ([redacted:<kind>]) and its order (PEM, connection
// URLs, cloud keys, bearer, JWT, provider tokens, CLI flags, key-value
// forms), and extends the provider and multilingual key tables for the
// Daemon's egress paths. RE2 has no lookahead, so length gates the upstream
// approximates with lookahead are checked in Go where noted.
package memory

import (
	"regexp"
	"strings"
)

// kvValue is the shared value class for key=value secrets: long,
// mostly-alphanumeric tokens with optional symbol runs.
const kvValue = `[A-Za-z0-9/+=._-]{16,}(?:[!#$%^*~@]+[A-Za-z0-9/+=._-]*)*`

var (
	pemPrivateRE = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY[A-Z0-9 ]*-----.*?-----END [A-Z0-9 ]*PRIVATE KEY[A-Z0-9 ]*-----`)
	// pemBreak is one line break inside a pasted block: a real newline or
	// the JSON-escaped \n of an event log line.
	pemBreak = `(?:[ \t]*\r?\n|(?:\\+r)?\\+n)[ \t]*`
	// pemPrivateOpenRE matches a truncated paste: the header, armour
	// headers and blank lines aside, plus base64 body lines with no END
	// line. A bare header with no body stays.
	pemPrivateOpenRE = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY[A-Z0-9 ]*-----` + `(?:` + pemBreak + `[A-Za-z][A-Za-z-]*: [^\r\n\\]*)*(?:` + pemBreak + `)+[A-Za-z0-9+/=]{16,}[ \t]*(?:` + pemBreak + `[A-Za-z0-9+/=]{16,}[ \t]*)*`)

	connURLRE = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9+.-]*://)([^\s/@:]*):([^\s]+)@([^\s]+)`)

	awsAccessKeyRE = regexp.MustCompile(`A(?:KIA|SIA)[0-9A-Z]{16}`)
	awsSecretRE    = regexp.MustCompile(`(?i)\b(aws[_-]?secret[_-]?access[_-]?key)(\\*['"]?\s*[:=]\s*)(\\*['"]?)([A-Za-z0-9/+=_-]{32,})(\\*['"]?)`)

	bearerRE    = regexp.MustCompile(`(?i)\b(Bearer|Basic)(\s+)([A-Za-z0-9._~+/=-]{16,})`)
	authTokenRE = regexp.MustCompile(`(?i)\b(authorization\\*["']?\s*[:=]\s*\\*["']?\s*token)(\s+)([A-Za-z0-9._~+/=-]{16,})`)

	jwtRE = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\b`)

	providerRE = regexp.MustCompile(`\b(gh[opsur]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{16,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9_-]*[A-Za-z0-9]{20,}[A-Za-z0-9_-]*|gsk_[A-Za-z0-9]{20,}|xai-[A-Za-z0-9]{20,}|hf_[A-Za-z0-9]{20,}|npm_[A-Za-z0-9]{30,}|xox[bpcs]-[A-Za-z0-9-]{10,}|AIza[0-9A-Za-z_-]{30,})\b`)
	// telegramBotRE needs a digit gate RE2 cannot express (5-16 digits):
	// the prefix length is rechecked in Go.
	telegramBotRE = regexp.MustCompile(`(?:^|[^0-9])([0-9]{5,20}:A[A-Za-z0-9_-]{34,})`)

	// passwordFlagRE catches CLI flags in --flag=value and --flag value
	// form. Space-separated prose after a flag ("--password from the
	// keychain") is skipped in Go.
	passwordFlagRE   = regexp.MustCompile(`(?i)(^|\s)(--?(?:password|passwd|pwd|passphrase|secret|token|api[_-]?key|auth[_-]?token|access[_-]?token|client[_-]?secret))([ =]\\*['"]?)([^\s'"]{3,128})`)
	passwordAssignRE = regexp.MustCompile(`(?i)\b((?:[\w.-]{0,64}?(?:password|passwd|pwd)|[\w.-]*[_.\-]pass))(\\*['"]?=\s*)(\\*['"]?)([^\s'"&]{3,128})(\\*['"]?)`)
	sshpassRE        = regexp.MustCompile(`(?i)(\bsshpass\b[^\n]{0,64}?-p[ =]?['"]?)([^\s'"]{3,})`)
	userPairRE       = regexp.MustCompile(`(?i)(?:^|\s)(?:-u|--user)[ =]['"]?([^\s:'"]{1,64}):([^\s'"]{3,})`)
	mysqlPassRE      = regexp.MustCompile(`(?i)(\b(?:mysql|mysqladmin|mysqldump|mariadb)\b[^\n]{0,120}?\s-p)([^\s'"-]\S{2,})`)
	loginPassRE      = regexp.MustCompile(`(?i)(\b(?:login|redis-cli|ftp|smbclient)\b[^\n]{0,120}?\s(?:-p|-a|--password|--with-token)[ =<]{1,6}['"]?)([^\s'"]{3,})`)
	netrcRE          = regexp.MustCompile(`(?i)(\b(?:machine|login)\s+\S+[^\n]{0,120}?\b(?:password|passwd)\s+)([^\s'"]{3,})`)

	genericKVRE = regexp.MustCompile(`(?i)\b([\w.-]{0,64}?(?:api[_-]?key|secret|token|passwd|password|authorization))(\\*['"]?\s*[:=]\s*)(\\*['"]?)(` + kvValue + `)(\\*['"]?)`)
	envKeyRE    = regexp.MustCompile(`\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*_KEY)(\\*['"]?\s*[:=]\s*)(\\*['"]?)(` + kvValue + `)(\\*['"]?)`)
	// intlKey alternates the non-English secret words the key-value
	// forms accept. Boundaries are explicit: \b is ASCII-only in RE2,
	intlKey = `парол[ьяею]|токен[ауы]?|секрет[ауы]?|ключ[аеиуом]?|contraseña|senha|passwort|kennwort|geheimnis|mot[\s_-]?de[\s_-]?passe|clé|jeton|clave|secreto|wachtwoord|sleutel|segreto|chiave|hasło|klucz|sekret|jelszó|titok|kulcs|şifre|sifre|anahtar|كلمة[\s_-]?السر|密码|密碼|パスワード|비밀번호|토큰`

	intlKVRE = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(` + intlKey + `)(\\*['"]?\s*[:=]\s*)(\\*['"]?)(` + kvValue + `)(\\*['"]?)`)
	// intlFillerKVRE allows up to 32 chars of filler between the key and
	// the separator ("пароль от стейджа: ..."), stopping at a bracket.
	intlFillerKVRE = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}_])(` + intlKey + `)([^\p{L}\n:=\[][^\n:=\[]{0,32}[:=]\s*)(\\*['"]?)(` + kvValue + `)(\\*['"]?)`)
	// prosePasswordRE catches "the password is hunter2..." sentences. The
	// value must hold a digit or symbol in Go, so bare prose ("the
	// password is wrong") stays.
	prosePasswordRE = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(password|passphrase|парол[ьяею])(\s+(?:is|was|это|будет)\s+)(\\*['"]?)([^\s'"]{8,128})`)
)

// unsecretValue reports values that look like placeholders, variable
// references or prose, never a secret.
func unsecretValue(v string) bool {
	if strings.Contains(v, "$") || strings.HasPrefix(v, "<") ||
		strings.HasPrefix(v, "%") || strings.HasPrefix(v, "{") ||
		strings.HasPrefix(v, "*") || strings.HasPrefix(v, "[redacted") {
		return true
	}
	switch strings.ToLower(v) {
	case "true", "false", "null", "none", "nil", "empty", "unset",
		"changeme", "password", "redacted", "redact", "***", "---":
		return true
	}
	return false
}

// proseFlagValue reports a lowercase word after a space-separated flag:
// "--password from the keychain" is prose, not a secret.
func proseFlagValue(sep, value string) bool {
	if strings.Contains(sep, "=") {
		return false
	}
	if len(value) > 12 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && r != '-' {
			return false
		}
	}
	return true
}

// secretProseValue reports an 8+ char prose value worth redacting: it must
// hold a digit or a symbol, so "the password is wrong" stays.
func secretProseValue(v string) bool {
	for _, r := range v {
		if (r >= '0' && r <= '9') || strings.ContainsRune("!#$%^*~@+/=._-", r) {
			return true
		}
	}
	return false
}

// RedactWithCounts replaces every recognized secret in s with a
// [redacted:<kind>] marker and counts the replacements by kind. It never
// errors and never drops non-secret text.
func RedactWithCounts(s string) (string, map[string]int) {
	counts := map[string]int{}
	if s == "" {
		return s, counts
	}
	add := func(kind string, n int) {
		if n > 0 {
			counts[kind] += n
		}
	}
	whole := func(re *regexp.Regexp, kind string) {
		n := 0
		s = re.ReplaceAllStringFunc(s, func(string) string {
			n++
			return "[redacted:" + kind + "]"
		})
		add(kind, n)
	}

	whole(pemPrivateRE, "private-key")
	whole(pemPrivateOpenRE, "private-key")

	n := 0
	s = connURLRE.ReplaceAllStringFunc(s, func(match string) string {
		n++
		m := connURLRE.FindStringSubmatch(match)
		return m[1] + m[2] + ":[redacted:url-credentials]@" + m[4]
	})
	add("url-credentials", n)

	n = 0
	s = awsSecretRE.ReplaceAllStringFunc(s, func(match string) string {
		n++
		m := awsSecretRE.FindStringSubmatch(match)
		return m[1] + m[2] + m[3] + "[redacted:aws-secret]" + closeQuote(m[3], m[5])
	})
	add("aws-secret", n)

	whole(awsAccessKeyRE, "aws-access-key")

	n = 0
	s = bearerRE.ReplaceAllStringFunc(s, func(match string) string {
		n++
		m := bearerRE.FindStringSubmatch(match)
		return m[1] + m[2] + "[redacted:bearer-token]"
	})
	add("bearer-token", n)

	n = 0
	s = authTokenRE.ReplaceAllStringFunc(s, func(match string) string {
		n++
		m := authTokenRE.FindStringSubmatch(match)
		return m[1] + m[2] + "[redacted:bearer-token]"
	})
	add("bearer-token", n)

	if strings.Contains(s, "eyJ") {
		whole(jwtRE, "jwt")
	}

	n = 0
	s = telegramBotRE.ReplaceAllStringFunc(s, func(match string) string {
		m := telegramBotRE.FindStringSubmatch(match)
		digits := m[1][:strings.Index(m[1], ":")]
		if len(digits) < 5 || len(digits) > 16 {
			return match
		}
		n++
		return strings.Replace(match, m[1], "[redacted:telegram-bot-token]", 1)
	})
	add("telegram-bot-token", n)

	s = providerRE.ReplaceAllStringFunc(s, func(v string) string {
		counts[providerKind(v)]++
		return "[redacted:" + providerKind(v) + "]"
	})

	n = 0
	s = passwordFlagRE.ReplaceAllStringFunc(s, func(match string) string {
		m := passwordFlagRE.FindStringSubmatch(match)
		if unsecretValue(m[4]) || proseFlagValue(m[3], m[4]) {
			return match
		}
		n++
		return m[1] + m[2] + m[3] + "[redacted:credential]"
	})
	add("credential", n)

	n = 0
	s = passwordAssignRE.ReplaceAllStringFunc(s, func(match string) string {
		m := passwordAssignRE.FindStringSubmatch(match)
		if unsecretValue(m[4]) {
			return match
		}
		n++
		return m[1] + m[2] + m[3] + "[redacted:password]" + closeQuote(m[3], m[5])
	})
	add("password", n)

	// Command-line secrets keep their command prefix: only the value is
	// replaced, so `mysqldump -p<secret> db` stays a mysqldump call.
	for _, cmd := range []struct {
		re   *regexp.Regexp
		kind string
	}{{sshpassRE, "command-password"}, {mysqlPassRE, "command-password"}, {loginPassRE, "command-password"}, {netrcRE, "password"}} {
		n = 0
		s = cmd.re.ReplaceAllStringFunc(s, func(match string) string {
			m := cmd.re.FindStringSubmatch(match)
			if unsecretValue(m[2]) {
				return match
			}
			n++
			return m[1] + "[redacted:" + cmd.kind + "]"
		})
		add(cmd.kind, n)
	}

	n = 0
	s = userPairRE.ReplaceAllStringFunc(s, func(match string) string {
		m := userPairRE.FindStringSubmatch(match)
		if allDigits(m[1]) && allDigits(m[2]) {
			return match // -u 1000:1000 is ids, not a login
		}
		n++
		return strings.Replace(match, m[1]+":"+m[2], m[1]+":[redacted:command-password]", 1)
	})
	add("command-password", n)

	n = 0
	s = genericKVRE.ReplaceAllStringFunc(s, func(match string) string {
		m := genericKVRE.FindStringSubmatch(match)
		if unsecretValue(m[4]) {
			return match
		}
		n++
		return m[1] + m[2] + m[3] + "[redacted:credential]" + closeQuote(m[3], m[5])
	})
	add("credential", n)

	n = 0
	s = envKeyRE.ReplaceAllStringFunc(s, func(match string) string {
		m := envKeyRE.FindStringSubmatch(match)
		if unsecretValue(m[4]) {
			return match
		}
		n++
		return m[1] + m[2] + m[3] + "[redacted:credential]" + closeQuote(m[3], m[5])
	})
	add("credential", n)

	for _, re := range []*regexp.Regexp{intlKVRE, intlFillerKVRE} {
		n = 0
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			m := re.FindStringSubmatch(match)
			if unsecretValue(m[5]) {
				return match
			}
			n++
			return m[1] + m[2] + m[3] + m[4] + "[redacted:credential]" + closeQuote(m[4], m[6])
		})
		add("credential", n)
	}

	n = 0
	s = prosePasswordRE.ReplaceAllStringFunc(s, func(match string) string {
		m := prosePasswordRE.FindStringSubmatch(match)
		value, trail := splitTrailPunct(m[4])
		if !secretProseValue(value) {
			return match
		}
		n++
		return strings.Replace(match, m[3]+m[4], m[3]+"[redacted:password]"+trail, 1)
	})
	add("password", n)

	return s, counts
}

func providerKind(v string) string {
	switch {
	case strings.HasPrefix(v, "ghp_"), strings.HasPrefix(v, "gho_"),
		strings.HasPrefix(v, "ghs_"), strings.HasPrefix(v, "ghu_"),
		strings.HasPrefix(v, "ghr_"), strings.HasPrefix(v, "github_pat_"):
		return "github-token"
	case strings.HasPrefix(v, "glpat-"):
		return "gitlab-token"
	case strings.HasPrefix(v, "sk_live_"), strings.HasPrefix(v, "sk_test_"),
		strings.HasPrefix(v, "rk_live_"), strings.HasPrefix(v, "rk_test_"):
		return "stripe-key"
	case strings.HasPrefix(v, "sk-ant-"):
		return "anthropic-key"
	case strings.HasPrefix(v, "sk-"):
		return "openai-key"
	case strings.HasPrefix(v, "gsk_"):
		return "groq-key"
	case strings.HasPrefix(v, "xai-"):
		return "xai-key"
	case strings.HasPrefix(v, "hf_"):
		return "huggingface-token"
	case strings.HasPrefix(v, "npm_"):
		return "npm-token"
	case strings.HasPrefix(v, "xoxb-"), strings.HasPrefix(v, "xoxp-"),
		strings.HasPrefix(v, "xoxc-"), strings.HasPrefix(v, "xoxs-"):
		return "slack-token"
	case strings.HasPrefix(v, "AIza"):
		return "google-api-key"
	}
	return "provider-token"
}

// Redact replaces every recognized secret in s with a [redacted:<kind>]
// marker. Model egresses use their role-specific wrapper, which all call this function.
func Redact(s string) string {
	out, _ := RedactWithCounts(s)
	return out
}

// EgressRole names a third-party or model egress the redactor guards.
type EgressRole string

const (
	EgressDistill  EgressRole = "distill"
	EgressDecider  EgressRole = "decider"
	EgressJev      EgressRole = "jev"
	EgressEmbedder EgressRole = "embedder"
)

// RedactedFor redacts s for the named egress role. Every role redacts the
// same way today; the role stays in the signature so a call site names what
// it sends to and a bypass scanner can find it.
func RedactedFor(_ EgressRole, s string) string { return Redact(s) }

// ForDistill redacts text bound for the distill LLM.
func ForDistill(s string) string { return RedactedFor(EgressDistill, s) }

// ForDecider redacts text bound for a Decider LLM.
func ForDecider(s string) string { return RedactedFor(EgressDecider, s) }

// ForEmbedder redacts text bound for the embedding provider.
func ForEmbedder(s string) string { return RedactedFor(EgressEmbedder, s) }

// ForJev redacts text bound for the Jev decision endpoint.
func ForJev(s string) string { return RedactedFor(EgressJev, s) }

// closeQuote re-emits the closing quote only when a quote was opened, so
// `"key": "value"` keeps its quotes and `key=value` gains none.
func closeQuote(open, close string) string {
	if !strings.Contains(open, "'") && !strings.Contains(open, `"`) {
		return ""
	}
	return close
}

// splitTrailPunct splits trailing sentence punctuation off a prose value,
// so "the password is hunter2-2026, rotate it" keeps its comma.
func splitTrailPunct(v string) (value, trail string) {
	value = strings.TrimRight(v, ",;:.!?")
	return value, v[len(value):]
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
