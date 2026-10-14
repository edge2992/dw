//go:build !unix

package summarize

import (
	"context"
	"errors"
	"time"
)

// The worker needs flock and setsid, which the standard library only offers
// on unix. Elsewhere the summarizer is unavailable and `dw hook` does nothing.

func lock(context.Context, string, time.Duration) (func(), error) {
	return nil, errors.ErrUnsupported
}

// Spawn is unsupported off unix.
func Spawn(Job) error { return errors.ErrUnsupported }
