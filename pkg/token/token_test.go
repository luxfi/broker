package token

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// slackBits is the number of unused low bits in the final base64url character
// for a payload of n bytes: 6*ceil(8n/6) - 8n. Each slack bit doubles the
// number of strings that decode to the same bytes.
func slackBits(n int) int { return 6*base64.RawURLEncoding.EncodedLen(n) - 8*n }

// respell returns every string that decodes to the same bytes as seg by
// varying only the non-significant low bits of its final character.
func respell(seg string, slack int) []string {
	last := strings.IndexByte(alphabet, seg[len(seg)-1])
	mask := 1<<slack - 1
	out := make([]string, 0, mask+1)
	for b := 0; b <= mask; b++ {
		out = append(out, seg[:len(seg)-1]+string(alphabet[(last&^mask)|b]))
	}
	return out
}

func TestSlackBitsMatchRealSignatureSizes(t *testing.T) {
	// The two signature sizes this repo verifies, and how many spellings a
	// lenient decoder would accept for each.
	for _, c := range []struct {
		name                    string
		bytes, slack, spellings int
	}{
		{"HMAC-SHA256", 32, 2, 4},
		{"RSA-2048 PKCS#1v1.5", 256, 4, 16},
	} {
		if got := slackBits(c.bytes); got != c.slack {
			t.Fatalf("%s: slack bits = %d, want %d", c.name, got, c.slack)
		}
		if 1<<c.slack != c.spellings {
			t.Fatalf("%s: spellings = %d, want %d", c.name, 1<<c.slack, c.spellings)
		}
	}
}

func TestDecodeAcceptsExactlyOneSpelling(t *testing.T) {
	for _, n := range []int{32, 256} {
		raw := make([]byte, n)
		for i := range raw {
			raw[i] = byte(i * 7)
		}
		canonical := Encode(raw)

		accepted := 0
		for _, s := range respell(canonical, slackBits(n)) {
			// Every spelling decodes to the same bytes under the lenient
			// decoder — that is the whole problem.
			lenient, err := base64.RawURLEncoding.DecodeString(s)
			if err != nil || string(lenient) != string(raw) {
				t.Fatalf("%d-byte: spelling %q does not decode to the same bytes; test premise is wrong", n, s[len(s)-4:])
			}
			if b, err := Decode(s); err == nil {
				accepted++
				if s != canonical {
					t.Errorf("%d-byte: accepted non-canonical spelling ...%q", n, s[len(s)-4:])
				}
				if string(b) != string(raw) {
					t.Errorf("%d-byte: canonical decode returned wrong bytes", n)
				}
			}
		}
		if accepted != 1 {
			t.Fatalf("%d-byte payload: %d spellings accepted, want exactly 1", n, accepted)
		}
	}
}

func TestDecodeRejectsNonCanonical(t *testing.T) {
	// 0xFB repeats as "-_" under base64url, so the URL alphabet is genuinely
	// exercised and the standard-alphabet vector below is not a no-op.
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = 0xFB
	}
	canonical := Encode(raw)
	if !strings.ContainsRune(canonical, '-') || !strings.ContainsRune(canonical, '_') {
		t.Fatalf("test premise: %q lacks URL-alphabet characters", canonical)
	}

	// Every case below decodes to raw under some standard-library decoder.
	// Strict() alone catches only the trailing-bits family; the whitespace
	// family is why Decode round-trips instead.
	for _, c := range []struct{ name, seg string }{
		{"trailing bits set", respell(canonical, 2)[1]},
		{"embedded LF", canonical[:10] + "\n" + canonical[10:]},
		{"embedded CRLF", canonical[:10] + "\r\n" + canonical[10:]},
		{"trailing LF", canonical + "\n"},
		{"leading LF", "\n" + canonical},
		{"padded", canonical + "="},
		{"standard alphabet", strings.NewReplacer("-", "+", "_", "/").Replace(canonical)},
	} {
		if _, err := Decode(c.seg); !errors.Is(err, ErrEncoding) {
			t.Errorf("%s: Decode accepted or returned %v, want ErrEncoding", c.name, err)
		}
	}

	if b, err := Decode(canonical); err != nil || string(b) != string(raw) {
		t.Fatalf("canonical segment rejected: %v", err)
	}
}

// The empty string is the canonical encoding of zero bytes, so the codec
// accepts it. Minimum length is a policy concern belonging to the caller —
// keeping it out of the codec is what stops the two from braiding. The JWT
// verifiers reject empty segments on their own terms; see the admin and auth
// packages.
func TestDecodeEmptyIsCanonicalZeroBytes(t *testing.T) {
	b, err := Decode("")
	if err != nil || len(b) != 0 {
		t.Fatalf("Decode(\"\") = %v, %v; want zero bytes and no error", b, err)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	for n := 0; n < 64; n++ {
		raw := make([]byte, n)
		for i := range raw {
			raw[i] = byte(255 - i)
		}
		b, err := Decode(Encode(raw))
		if err != nil || string(b) != string(raw) {
			t.Fatalf("n=%d: round trip failed: %v", n, err)
		}
	}
}

func TestIDIsStableAndNotTheToken(t *testing.T) {
	tok := Encode([]byte("a bearer secret"))
	id := ID(tok)
	if id != ID(tok) {
		t.Fatal("ID is not deterministic")
	}
	if len(id) != 64 {
		t.Fatalf("ID length = %d, want 64 hex chars", len(id))
	}
	if strings.Contains(id, tok) {
		t.Fatal("ID leaks the token it identifies")
	}
	if ID(tok) == ID(tok+"x") {
		t.Fatal("distinct tokens share an ID")
	}
}
