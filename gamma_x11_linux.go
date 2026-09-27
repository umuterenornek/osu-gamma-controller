package main

// Gamma control for X11 through RandR CRTC gamma ramps (what xgamma and redshift use).

import (
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

type x11Crtc struct {
	id                  randr.Crtc
	size                uint16
	origR, origG, origB []uint16
}

type x11Gamma struct {
	conn  *xgb.Conn
	crtcs []x11Crtc
}

func newX11Gamma() (*x11Gamma, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connecting to X server: %w", err)
	}
	g := &x11Gamma{conn: conn}
	if err := g.init(); err != nil {
		conn.Close()
		return nil, err
	}
	return g, nil
}

func (g *x11Gamma) init() error {
	if err := randr.Init(g.conn); err != nil {
		return fmt.Errorf("X server has no RandR extension: %w", err)
	}
	root := xproto.Setup(g.conn).DefaultScreen(g.conn).Root
	res, err := randr.GetScreenResourcesCurrent(g.conn, root).Reply()
	if err != nil {
		return err
	}
	for _, id := range res.Crtcs {
		info, err := randr.GetCrtcInfo(g.conn, id, res.ConfigTimestamp).Reply()
		if err != nil || info.Mode == 0 {
			continue // inactive CRTC
		}
		size, err := randr.GetCrtcGammaSize(g.conn, id).Reply()
		if err != nil || size.Size == 0 {
			continue
		}
		orig, err := randr.GetCrtcGamma(g.conn, id).Reply()
		if err != nil {
			return err
		}
		g.crtcs = append(g.crtcs, x11Crtc{id: id, size: size.Size, origR: orig.Red, origG: orig.Green, origB: orig.Blue})
	}
	if len(g.crtcs) == 0 {
		return errors.New("no active RandR CRTC supports gamma ramps")
	}
	return nil
}

func (g *x11Gamma) Name() string { return "RandR (X11)" }

func (g *x11Gamma) Set(gamma float64) error {
	if err := validateGamma(gamma); err != nil {
		return err
	}
	for _, c := range g.crtcs {
		ramp := buildRamp(int(c.size), gamma)
		if err := randr.SetCrtcGammaChecked(g.conn, c.id, c.size, ramp, ramp, ramp).Check(); err != nil {
			return err
		}
	}
	return nil
}

func (g *x11Gamma) Close() error {
	var errs []error
	for _, c := range g.crtcs {
		errs = append(errs, randr.SetCrtcGammaChecked(g.conn, c.id, c.size, c.origR, c.origG, c.origB).Check())
	}
	g.conn.Close()
	return errors.Join(errs...)
}
