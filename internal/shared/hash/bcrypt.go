package hash

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const BcryptCost = 12 // per SRS section 4.2 Security

func Password(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), BcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ComparePassword(hashStr, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashStr), []byte(plain)) == nil
}

// SHA256Hex is used for storing refresh tokens (we never want plaintext in DB)
// and for hashing PII (email/phone) in deleted_accounts audit trail.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
