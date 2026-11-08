//go:build !linux && !darwin

package chatgpt

import (
	"context"
	"errors"
)

func lockProtection(context.Context) (func(), error) {
	return nil, errors.New("cross-process protection locking is unsupported on this platform")
}
