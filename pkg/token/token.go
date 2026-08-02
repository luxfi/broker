// Package token defines the one canonical text encoding of a bearer
// credential: unpadded base64url, and nothing else.
//
// A bearer token is a byte string; its text is transport. Whatever stores,
// compares, revokes, audits, dedups or rate-limits a credential must key on
// the bytes — or on ID — never on the string a client submitted. When one
// credential can wear several spellings, every string-keyed structure
// silently splits it into several principals: a denylist misses the
// spellings it was not given, an audit log reads one actor as many, and a
// rate limiter multiplies the budget by the size of the equivalence class.
package token

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

// ErrEncoding reports text that is not the canonical encoding of its bytes.
var ErrEncoding = errors.New("token: non-canonical encoding")

var enc = base64.RawURLEncoding

// Decode returns the bytes of a canonically encoded segment and rejects
// every other spelling of those same bytes.
//
// Go's base64 decoders are lenient in two independent ways. The unused low
// bits of the final quantum are ignored — an HS256 signature is 32 bytes in
// 43 characters, leaving 2 slack bits and so 4 spellings; an RS256 signature
// is 256 bytes in 342 characters, leaving 4 slack bits and so 16 spellings.
// And \r and \n are skipped wherever they appear, which admits unboundedly
// many more over any carrier that permits them. Encoding.Strict() closes
// only the first of those, so it is not a fix on its own.
//
// Round-tripping closes both, and anything else of the same shape, with a
// single predicate: a segment is canonical exactly when it is the encoding
// of the bytes it decodes to. There is nothing to enumerate and nothing to
// keep in sync as the standard library's leniency changes.
//
// Non-canonical input is rejected, never normalized. A client that sends one
// is buggy or probing; accepting it quietly is how the malleability survives
// the next refactor.
func Decode(seg string) ([]byte, error) {
	b, err := enc.DecodeString(seg)
	if err != nil || enc.EncodeToString(b) != seg {
		return nil, ErrEncoding
	}
	return b, nil
}

// Encode is the inverse of Decode and the only encoder for token text.
func Encode(b []byte) string { return enc.EncodeToString(b) }

// ID is the identity of a token: hex SHA-256 over its bytes. Key revocation
// lists, audit records, dedup caches and rate limiters on this. Never on the
// token itself — that is the bearer secret, and it must not reach a log line
// or a database column in the clear.
//
// Pass only a token that validation has already accepted: validation is what
// establishes that the text is the unique encoding of its bytes, and hence
// that equal credentials have equal IDs.
func ID(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}
