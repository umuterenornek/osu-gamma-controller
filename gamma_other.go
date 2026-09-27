//go:build !linux && !windows

package main

import (
	"fmt"
	"runtime"
)

func newGammaSetter(backend string) (GammaSetter, error) {
	return nil, fmt.Errorf("gamma control is not supported on %s", runtime.GOOS)
}
