package sentinel

import (
	"encoding/hex"
	"testing"
)

func TestParseDifficulty(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "0"},
		{"0fffff", "0fffff"},
		{"0FfFfF", "0fffff"},
		{"00", "00"},
	}
	for _, c := range cases {
		got := parseDifficulty(c.in)
		if got.raw != c.want {
			t.Errorf("parseDifficulty(%q).raw = %q, want %q", c.in, got.raw, c.want)
		}
	}
}

func TestMatchesTrivialDifficulty(t *testing.T) {
	fnv := FNV1a32("seed-data")
	digest, _ := hex.DecodeString(fnv)
	// Impossible difficulty should not match.
	if matches(digest, parseDifficulty("00000000")) {
		t.Errorf("matches(%s) should be false for difficulty 00000000", fnv)
	}
	// Fully permissive difficulty should match.
	if !matches(digest, parseDifficulty("ffffffff")) {
		t.Errorf("matches(%s) should be true for difficulty ffffffff", fnv)
	}
}

func TestSolveProofFNVAndSHA3(t *testing.T) {
	g := NewGenerator("test-device-id", "")
	// Permissive difficulty within the 32-bit FNV digest range.
	if _, ok := g.SolveProof(PoWUseFNV, "seed191919", "ffffffff", 10000); !ok {
		t.Errorf("FNV solve should succeed with permissive difficulty")
	}
	// SHA3-512 has a 64-byte digest, so a longer difficulty is satisfiable.
	if _, ok := g.SolveProof(PoWUseSHA3, "seed191919", "ffffffffffffff", 10000); !ok {
		t.Errorf("SHA3 solve should succeed with permissive difficulty")
	}
}

func TestHashForFNVMatch(t *testing.T) {
	fnv := FNV1a32("abc")
	digest := hashFor(PoWUseFNV, "abc")
	if hex.EncodeToString(digest) != fnv {
		t.Errorf("hashFor(FNV) digest mismatch: got %x want %s", digest, fnv)
	}
}

func TestHashForSHA3Deterministic(t *testing.T) {
	a := hashFor(PoWUseSHA3, "abc")
	b := hashFor(PoWUseSHA3, "abc")
	if string(a) != string(b) {
		t.Errorf("SHA3 hash should be deterministic")
	}
	if len(a) != 64 {
		t.Errorf("SHA3-512 digest length = %d, want 64", len(a))
	}
}