package main

import (
	"fmt"
	"math"
)

// GammaSetter changes the gamma of every connected display.
//
// Gamma values follow xgamma semantics: each channel is mapped as
// output = input^(1/gamma), so values above 1 brighten and values below 1 darken.
type GammaSetter interface {
	Name() string
	Set(gamma float64) error
	// Close restores the displays to the state they were in before the setter was created.
	Close() error
}

// buildRamp returns a gamma ramp with the given number of entries, scaled to 0..65535.
func buildRamp(size int, gamma float64) []uint16 {
	ramp := make([]uint16, size)
	if size < 2 {
		return ramp
	}
	for i := range ramp {
		v := math.Pow(float64(i)/float64(size-1), 1/gamma)
		ramp[i] = uint16(math.Round(math.Min(math.Max(v, 0), 1) * 65535))
	}
	return ramp
}

func validateGamma(gamma float64) error {
	if !(gamma > 0) || math.IsInf(gamma, 0) {
		return fmt.Errorf("invalid gamma value %v: must be a positive number", gamma)
	}
	return nil
}
