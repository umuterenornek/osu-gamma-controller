package main

// Gamma control for Windows through GDI's SetDeviceGammaRamp, applied to every display
// attached to the desktop.
//
// By default Windows rejects ramps that stray far from identity. The extreme values osu! players
// tend to use need the GdiIcmGammaRange registry value set to 256 (see README).

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	gdi32                   = syscall.NewLazyDLL("gdi32.dll")
	procEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	procCreateDCW           = gdi32.NewProc("CreateDCW")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procGetDeviceGammaRamp  = gdi32.NewProc("GetDeviceGammaRamp")
	procSetDeviceGammaRamp  = gdi32.NewProc("SetDeviceGammaRamp")
)

const displayDeviceAttachedToDesktop = 0x1

type displayDevice struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

type gammaRamp [3][256]uint16

type winDisplay struct {
	name string
	hdc  uintptr
	orig gammaRamp
}

type winGamma struct {
	displays []winDisplay
}

func newGammaSetter(backend string) (GammaSetter, error) {
	switch backend {
	case "", "auto", "gdi":
		return newWinGamma()
	}
	return nil, fmt.Errorf("unknown gamma backend %q (valid on Windows: auto, gdi)", backend)
}

func newWinGamma() (*winGamma, error) {
	g := &winGamma{}
	for i := uint32(0); ; i++ {
		var dd displayDevice
		dd.cb = uint32(unsafe.Sizeof(dd))
		if ok, _, _ := procEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0); ok == 0 {
			break
		}
		if dd.stateFlags&displayDeviceAttachedToDesktop == 0 {
			continue
		}
		hdc, _, _ := procCreateDCW.Call(0, uintptr(unsafe.Pointer(&dd.deviceName[0])), 0, 0)
		if hdc == 0 {
			continue
		}
		d := winDisplay{name: syscall.UTF16ToString(dd.deviceName[:]), hdc: hdc}
		if ok, _, _ := procGetDeviceGammaRamp.Call(hdc, uintptr(unsafe.Pointer(&d.orig))); ok == 0 {
			procDeleteDC.Call(hdc)
			continue
		}
		g.displays = append(g.displays, d)
	}
	if len(g.displays) == 0 {
		return nil, errors.New("no display supports gamma ramps")
	}
	return g, nil
}

func (g *winGamma) Name() string { return "GDI gamma ramp (Windows)" }

func (g *winGamma) Set(gamma float64) error {
	if err := validateGamma(gamma); err != nil {
		return err
	}
	var ramp gammaRamp
	values := buildRamp(256, gamma)
	for ch := range ramp {
		copy(ramp[ch][:], values)
	}
	var errs []error
	for _, d := range g.displays {
		if ok, _, _ := procSetDeviceGammaRamp.Call(d.hdc, uintptr(unsafe.Pointer(&ramp))); ok == 0 {
			errs = append(errs, fmt.Errorf("%s rejected gamma %v; Windows limits the gamma range unless "+
				`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ICM\GdiIcmGammaRange is set to 256`, d.name, gamma))
		}
	}
	return errors.Join(errs...)
}

func (g *winGamma) Close() error {
	var errs []error
	for _, d := range g.displays {
		if ok, _, _ := procSetDeviceGammaRamp.Call(d.hdc, uintptr(unsafe.Pointer(&d.orig))); ok == 0 {
			errs = append(errs, fmt.Errorf("%s: failed to restore original gamma ramp", d.name))
		}
		procDeleteDC.Call(d.hdc)
	}
	return errors.Join(errs...)
}
