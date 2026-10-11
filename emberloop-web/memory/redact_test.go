package memory

import (
	"testing"
)

// redactCases are golden pairs per pattern family: input redacts to want.
// Secrets are synthetic; the shapes mirror real tokens.
var redactCases = []struct {
	name  string
	in    string
	want  string
	kinds []string // count keys RedactWithCounts must report
}{
	{"aws access key", "key AKIAIOSFODNN7EXAMPLE here", "key [redacted:aws-access-key] here", []string{"aws-access-key"}},
	{"aws temp key", "key ASIAIOSFODNN7EXAMPLE here", "key [redacted:aws-access-key] here", []string{"aws-access-key"}},
	{"aws secret", "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_secret_access_key = [redacted:aws-secret]", []string{"aws-secret"}},
	{"github token", "push with gh" + "p_aBcDeFgHiJkLmNoPqRsT0123456789ABCD now", "push with [redacted:github-token] now", []string{"github-token"}},
	{"github oauth", "use gh" + "o_aBcDeFgHiJkLmNoPqRsT0123456789ABCD now", "use [redacted:github-token] now", []string{"github-token"}},
	{"openai key", "key is sk" + "-proj-aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "key is [redacted:openai-key] ok", []string{"openai-key"}},
	{"anthropic key", "key is sk" + "-ant-aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "key is [redacted:anthropic-key] ok", []string{"anthropic-key"}},
	{"gitlab token", "use gl" + "pat-aBcDeFgHiJkLmNoPqRsT0123 now", "use [redacted:gitlab-token] now", []string{"gitlab-token"}},
	{"stripe key", "charge sk" + "_live_aBcDeFgHiJkLmNoPqRsT0123456 now", "charge [redacted:stripe-key] now", []string{"stripe-key"}},
	{"groq key", "key gs" + "k_aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "key [redacted:groq-key] ok", []string{"groq-key"}},
	{"xai key", "key xa" + "i-aBcDeFgHiJkLmNoPqRsT0123456789 ok", "key [redacted:xai-key] ok", []string{"xa" + "i-key"}},
	{"huggingface", "token hf" + "_aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "token [redacted:huggingface-token] ok", []string{"huggingface-token"}},
	{"npm token", "publish np" + "m_aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "publish [redacted:npm-token] ok", []string{"npm-token"}},
	{"slack token", "post xo" + "xb-123456789012-aBcDeFgHiJkLmNoPqRsT now", "post [redacted:slack-token] now", []string{"slack-token"}},
	{"google key", "map AI" + "zaaBcDeFgHiJkLmNoPqRsT0123456789ABCD ok", "map [redacted:google-api-key] ok", []string{"google-api-key"}},
	{"telegram bot", "call 123456789:AABCDEFGHIJKLMNOPQRSTUVWXYZ123456789 now", "call [redacted:telegram-bot-token] now", []string{"telegram-bot-token"}},
	{
		"jwt",
		"token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJVadQssw5c ok",
		"token [redacted:jwt] ok",
		[]string{"jwt"},
	},
	{
		"connection url",
		"db postgres://admin:Sup3rS3cretValue123@db.example.com:5432/app here",
		"db postgres://admin:[redacted:url-credentials]@db.example.com:5432/app here",
		[]string{"url-credentials"},
	},

	{
		"bearer",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9abcdefgh123456 ok",
		"Authorization: Bearer [redacted:bearer-token] ok",
		[]string{"bearer-token"},
	},
	{
		"basic",
		"Authorization: Basic YWRtaW46UzNjcjN0UEBzczB4cmQh ok",
		"Authorization: Basic [redacted:bearer-token] ok",
		[]string{"bearer-token"},
	},
	{
		"authorization token",
		"authorization: token gh" + "p_aBcDeFgHiJkLmNoPqRsT0123456789ABCD ok",
		"authorization: token [redacted:bearer-token] ok",
		[]string{"bearer-token"},
	},
	{"cli equals", "mycli --password=Sup3rS3cretValue123 run", "mycli --password=[redacted:credential] run", []string{"credential"}},
	{"cli space", "mycli --token hunter2-2026 run", "mycli --token [redacted:credential] run", []string{"credential"}},
	{"cli api key", "deploy --api-key ABC123xyz789DEF456ghi run", "deploy --api-key [redacted:credential] run", []string{"credential"}},
	{"cli prose kept", "get --password from the keychain", "get --password from the keychain", nil},
	{"env assign", "DATABASE_PASSWORD=DotenvPass2026xyz", "DATABASE_PASSWORD=[redacted:password]", []string{"password"}},
	{"pass suffix", "DB_PASS=K8sPass2026xyz", "DB_PASS=[redacted:password]", []string{"password"}},
	{"placeholder kept", "PASSWORD=$DATABASE_PASSWORD", "PASSWORD=$DATABASE_PASSWORD", nil},
	{"bool kept", "secret=false", "secret=false", nil},
	{"sshpass", "sshpass -p hunter2-2026 ssh user@host", "sshpass -p [redacted:command-password] ssh user@host", []string{"command-password"}},
	{"user pair", "curl -u admin:hunter2-2026 https://example.com", "curl -u admin:[redacted:command-password] https://example.com", []string{"command-password"}},
	{"user ids kept", "docker run -u 1000:1000 img", "docker run -u 1000:1000 img", nil},
	{"mysql", "mysqldump -pMyRootPass2026 db", "mysqldump -p[redacted:command-password] db", []string{"command-password"}},
	{"docker login", "docker login -p Sup3rS3cretValue123", "docker login -p [redacted:command-password]", []string{"command-password"}},
	{"netrc", "machine api.example.com login deploy password hunter2-2026", "machine api.example.com login deploy password [redacted:password]", []string{"password"}},
	{"generic kv", "api_key = ABCDEFGHIJKLMNOP1234567890abcd", "api_key = [redacted:credential]", []string{"credential"}},
	{"env key", "export GROQ_KEY=ABCDEF1234567890abcdef12", "export GROQ_KEY=[redacted:credential]", []string{"credential"}},
	{"quoted kv", `"x-api-key": "ABCDEFGHIJKLMNOP1234567890abcd"`, `"x-api-key": "[redacted:credential]"`, []string{"credential"}},
	{"german", "passwort: hunter2supersecretvalue123", "passwort: [redacted:credential]", []string{"credential"}},
	{"spanish", "contraseña: ABCDEF1234567890abcdef12", "contraseña: [redacted:credential]", []string{"credential"}},
	{"portuguese", "senha = ABCDEF1234567890abcdef12", "senha = [redacted:credential]", []string{"credential"}},
	{"french", "mot de passe: ABCDEF1234567890abcdef12", "mot de passe: [redacted:credential]", []string{"credential"}},
	{"russian", "пароль: ABCDEF1234567890abcdef12", "пароль: [redacted:credential]", []string{"credential"}},
	{"russian filler", "пароль от стейджа: ABCDEF1234567890abcdef12", "пароль от стейджа: [redacted:credential]", []string{"credential"}},
	{"chinese", "密码: ABCDEF1234567890abcdef12", "密码: [redacted:credential]", []string{"credential"}},
	{"japanese", "パスワード: ABCDEF1234567890abcdef12", "パスワード: [redacted:credential]", []string{"credential"}},
	{"prose secret", "the admin password is hunter2-2026, rotate it", "the admin password is [redacted:password], rotate it", []string{"password"}},
	{"prose bare kept", "the password is wrong, try again", "the password is wrong, try again", nil},
	{
		"pem whole",
		"key:\n-----BEGIN RSA PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEINT8h5R7mABCDEF1234567890abcdef\n-----END RSA PRIVATE KEY-----\ndone",
		"key:\n[redacted:private-key]\ndone",
		[]string{"private-key"},
	},
	{
		"pem truncated",
		"half:\n-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEINT8h5R7mABCDEF1234567890abcdef\nABCD1234567890abcdef1234567890ABCD1234",
		"half:\n[redacted:private-key]",
		[]string{"private-key"},
	},
	{"pem header kept", "has -----BEGIN PRIVATE KEY----- only", "has -----BEGIN PRIVATE KEY----- only", nil},
	{"plain kept", "always wrap errors in this repo", "always wrap errors in this repo", nil},
}

func TestRedactGolden(t *testing.T) {
	for _, c := range redactCases {
		got, counts := RedactWithCounts(c.in)
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
		for _, k := range c.kinds {
			if counts[k] < 1 {
				t.Errorf("%s: counts %v lack kind %q", c.name, counts, k)
			}
		}
		if c.kinds == nil && len(counts) != 0 {
			t.Errorf("%s: kept text counted %v", c.name, counts)
		}
	}
}
func TestEgressWrappersRedact(t *testing.T) {
	in := "token gh" + "p_aBcDeFgHiJkLmNoPqRsT0123456789ABCD with passwort: hunter2supersecretvalue123"
	want := Redact(in)
	for name, fn := range map[string]func(string) string{
		"ForDistill": ForDistill,
		"ForDecider": ForDecider,
		"ForJev":     ForJev,
	} {
		if got := fn(in); got != want {
			t.Errorf("%s = %q, want redacted %q", name, got, want)
		}
	}
	if got := RedactedFor(EgressJev, in); got != want {
		t.Errorf("RedactedFor(jev) = %q, want %q", got, want)
	}
}
