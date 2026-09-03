// Package httperr holds the error sentinels the forge clients under
// internal/ share, so a status check has one type to wrap regardless of
// which forge's response it came from -- the same reasoning httpauth
// already gives for sharing its own encoding.
package httperr

import "errors"

// ErrUnexpectedStatus means a forge's API returned a status code the
// client didn't expect for the request it made.
var ErrUnexpectedStatus = errors.New("unexpected status")

// ErrUnauthenticatedRedirect means a request that should have carried
// authentication got redirected to a sign-in page instead of the resource
// it asked for -- a 200 OK that isn't the response it looks like.
var ErrUnauthenticatedRedirect = errors.New("unauthenticated redirect")
