package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
)

func newGammaSetter(backend string) (GammaSetter, error) {
	switch backend {
	case "", "auto":
		return detectGammaSetter()
	case "wlr":
		return newWlrGamma()
	case "kwin":
		return newKWinGamma()
	case "x11":
		return newX11Gamma()
	}
	return nil, fmt.Errorf("unknown gamma backend %q (valid on Linux: auto, wlr, kwin, x11)", backend)
}

func detectGammaSetter() (GammaSetter, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		g, err := newWlrGamma()
		if err == nil {
			return g, nil
		}
		if !errors.Is(err, errWlrUnsupported) {
			return nil, err
		}
		if isDesktop("KDE") {
			log.Println("Compositor has no wlr-gamma-control support, using KWin ICC profiles instead")
			return newKWinGamma()
		}
		if isDesktop("GNOME") {
			return nil, errors.New("GNOME (Mutter) on Wayland does not expose any gamma control to applications")
		}
		return nil, err
	}
	if os.Getenv("DISPLAY") != "" {
		return newX11Gamma()
	}
	return nil, errors.New("no Wayland or X11 display found (WAYLAND_DISPLAY and DISPLAY are unset)")
}

func isDesktop(name string) bool {
	for _, d := range strings.Split(os.Getenv("XDG_CURRENT_DESKTOP"), ":") {
		if strings.EqualFold(d, name) {
			return true
		}
	}
	return false
}
