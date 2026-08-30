// Package httpauth builds Authorization header values the forge clients
// under internal/ share, so the encoding lives in one place rather than a
// copy per adapter.
package httpauth

import "encoding/base64"

// Basic returns the base64-encoded "user:password" pair HTTP Basic auth
// expects, without the "Basic " scheme prefix -- callers add that
// themselves, since some also need to set other schemes ("token <t>") the
// same way.
func Basic(user, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
}
