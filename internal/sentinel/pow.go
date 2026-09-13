package sentinel

import (
	"encoding/hex"
	"time"

	"golang.org/x/crypto/sha3"
)

// PoWHash identifies the hash function used to solve a Sentinel proof of work.
type PoWHash int

const (
	// PoWUnknown means no hash has been selected yet.
	PoWUnknown PoWHash = iota
	// PoWUseFNV selects the FNV-1a + murmur3-style avalanche finalizer.
	PoWUseFNV
	// PoWUseSHA3 selects SHA3-512 as used by the reference Python implementation.
	PoWUseSHA3
)

// parsedDifficulty is a normalized difficulty spec.
type parsedDifficulty struct {
	raw string
}

// parseDifficulty normalizes a difficulty string to lowercase hex. Reference
// implementations supply hex like "0fffff"; the empty value defaults to "0".
func parseDifficulty(d string) parsedDifficulty {
	if d == "" {
		d = "0"
	}
	return parsedDifficulty{raw: toLowerHex(d)}
}

func toLowerHex(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'F' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// matches reports whether a computed digest satisfies the difficulty. The
// digest is compared as lowercase hex against the leading difficulty nibbles,
// which is faithful for both the 32-bit FNV digest and a SHA3-512 digest. A
// difficulty longer than the digest can supply can never be satisfied.
func matches(digest []byte, d parsedDifficulty) bool {
	h := hex.EncodeToString(digest)
	if len(h) < len(d.raw) {
		return false
	}
	return h[:len(d.raw)] <= d.raw
}

// hashFor applies the selected hash function and returns its digest bytes.
func hashFor(kind PoWHash, input string) []byte {
	switch kind {
	case PoWUseFNV:
		h, _ := hex.DecodeString(FNV1a32(input))
		return h
	case PoWUseSHA3:
		sum := sha3.Sum512([]byte(input))
		return sum[:]
	default:
		// Neutral default that never matches under normal difficulty.
		return make([]byte, 64)
	}
}

// SolveProof iterates the config nonce until a digest satisfies the difficulty
// using the given hash. It returns (proofData, ok).
func (g *SentinelTokenGenerator) SolveProof(kind PoWHash, seed, difficulty string, maxIter int) (string, bool) {
	if seed == "" {
		seed = g.RequirementsSeed
	}
	if maxIter <= 0 {
		maxIter = 500000
	}
	d := parseDifficulty(difficulty)
	start := time.Now()
	config := g.getConfig()

	for i := 0; i < maxIter; i++ {
		config[3] = i
		config[9] = time.Since(start).Milliseconds()

		data := g.base64Encode(config)
		digest := hashFor(kind, seed+data)

		if matches(digest, d) {
			return data, true
		}
	}
	return "", false
}