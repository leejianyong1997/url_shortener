package shortener

import (
	"crypto/rand"
	"math/big"
)

// alphabet is the base62 character set: 0-9, a-z, A-Z = 62 symbols.
// 62 is chosen because these are exactly the characters that are safe and
// readable in a URL path without encoding.
const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateCode returns a random base62 string of length n.
//
// Design notes (the part worth explaining in an interview):
//
//   - We use crypto/rand, NOT math/rand. math/rand is seeded and predictable:
//     if an attacker knows the seed they can predict future codes and guess
//     other people's short links. crypto/rand reads from the OS CSPRNG, so the
//     codes are unguessable. For a public URL shortener, unpredictability is a
//     real security property, so the extra cost is justified.
//
//   - We pick each character independently and uniformly from the 62-symbol
//     alphabet. rand.Int(reader, 62) returns a value in [0,62) with NO modulo
//     bias (a naive `b % 62` over raw bytes would slightly favor smaller
//     indexes). Uniform distribution keeps the full keyspace usable.
//
//   - Keyspace: 62^n. For n=7 that is ~3.5 trillion combinations, so random
//     collisions are extremely rare — but "rare" is not "never", which is why
//     the caller (Service.Shorten) still handles collisions. See service.go.
func GenerateCode(n int) (string, error) {
	max := big.NewInt(int64(len(alphabet))) // 62
	b := make([]byte, n)
	for i := range b {
		// idx is a cryptographically secure random integer in [0, 62).
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b), nil
}
