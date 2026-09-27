package main

// Gamma control for wlroots-style Wayland compositors (Sway, Hyprland, niri, river, Wayfire, labwc, ...)
// through the wlr-gamma-control-unstable-v1 protocol. This is a minimal hand-written Wayland client
// that only speaks the handful of messages the protocol needs, so no libwayland or cgo is required.
//
// The compositor restores the original gamma as soon as our connection closes, including when the
// process crashes, so there is nothing to clean up on exit beyond closing the socket.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var errWlrUnsupported = errors.New("compositor does not support wlr-gamma-control-unstable-v1")

var ne = binary.NativeEndian

const (
	wlDisplayID = 1

	// wl_display requests / events
	wlDisplaySync        = 0
	wlDisplayGetRegistry = 1
	wlDisplayError       = 0

	// wl_registry
	wlRegistryBind         = 0
	wlRegistryGlobal       = 0
	wlRegistryGlobalRemove = 1

	// wl_output
	wlOutputName = 4

	// zwlr_gamma_control_manager_v1 / zwlr_gamma_control_v1
	wlrGammaManagerGetControl = 0
	wlrGammaControlSetGamma   = 0
	wlrGammaControlDestroy    = 1
	wlrGammaControlGammaSize  = 0
	wlrGammaControlFailed     = 1
)

type wlHandler func(op uint16, args *wlArgs) error

type wlClient struct {
	conn     *net.UnixConn
	nextID   uint32
	rbuf     []byte
	handlers map[uint32]wlHandler
}

func wlConnect() (*wlClient, error) {
	name := os.Getenv("WAYLAND_DISPLAY")
	if name == "" {
		name = "wayland-0"
	}
	path := name
	if !filepath.IsAbs(path) {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			return nil, errors.New("XDG_RUNTIME_DIR is not set")
		}
		path = filepath.Join(dir, name)
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("connecting to Wayland display: %w", err)
	}
	return &wlClient{conn: conn, nextID: 2, handlers: map[uint32]wlHandler{}}, nil
}

func (c *wlClient) newID() uint32 {
	id := c.nextID
	c.nextID++
	return id
}

// send writes one request. fd, if >= 0, is passed alongside it via SCM_RIGHTS.
func (c *wlClient) send(obj uint32, op uint16, args []byte, fd int) error {
	msg := make([]byte, 8, 8+len(args))
	ne.PutUint32(msg[0:], obj)
	ne.PutUint32(msg[4:], uint32(8+len(args))<<16|uint32(op))
	msg = append(msg, args...)
	var oob []byte
	if fd >= 0 {
		oob = syscall.UnixRights(fd)
	}
	_, _, err := c.conn.WriteMsgUnix(msg, oob, nil)
	return err
}

func (c *wlClient) readMsg() (obj uint32, op uint16, args []byte, err error) {
	for {
		if len(c.rbuf) >= 8 {
			size := int(ne.Uint32(c.rbuf[4:]) >> 16)
			if size < 8 {
				return 0, 0, nil, fmt.Errorf("malformed Wayland message (size %d)", size)
			}
			if len(c.rbuf) >= size {
				obj = ne.Uint32(c.rbuf[0:])
				op = uint16(ne.Uint32(c.rbuf[4:]))
				args = append([]byte(nil), c.rbuf[8:size]...)
				c.rbuf = c.rbuf[size:]
				return obj, op, args, nil
			}
		}
		buf := make([]byte, 4096)
		n, err := c.conn.Read(buf)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("reading from Wayland display: %w", err)
		}
		c.rbuf = append(c.rbuf, buf[:n]...)
	}
}

func (c *wlClient) dispatch() error {
	obj, op, payload, err := c.readMsg()
	if err != nil {
		return err
	}
	args := &wlArgs{b: payload}
	if obj == wlDisplayID {
		if op == wlDisplayError {
			id, code, msg := args.uint(), args.uint(), args.string()
			return fmt.Errorf("Wayland protocol error on object %d (code %d): %s", id, code, msg)
		}
		return nil // delete_id: we never reuse ids, nothing to do
	}
	if h := c.handlers[obj]; h != nil {
		if err := h(op, args); err != nil {
			return err
		}
		return args.err
	}
	return nil
}

// roundtrip blocks until the compositor has processed every request sent so far,
// dispatching all events that arrive in the meantime.
func (c *wlClient) roundtrip() error {
	id := c.newID()
	done := false
	c.handlers[id] = func(uint16, *wlArgs) error { done = true; return nil }
	defer delete(c.handlers, id)
	if err := c.send(wlDisplayID, wlDisplaySync, wlUint(nil, id), -1); err != nil {
		return err
	}
	for !done {
		if err := c.dispatch(); err != nil {
			return err
		}
	}
	return nil
}

func (c *wlClient) bind(registry, name uint32, iface string, version uint32) (uint32, error) {
	id := c.newID()
	args := wlUint(nil, name)
	args = wlString(args, iface)
	args = wlUint(args, version)
	args = wlUint(args, id)
	return id, c.send(registry, wlRegistryBind, args, -1)
}

func wlUint(b []byte, v uint32) []byte { return ne.AppendUint32(b, v) }

func wlString(b []byte, s string) []byte {
	b = ne.AppendUint32(b, uint32(len(s)+1))
	b = append(b, s...)
	b = append(b, 0)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

type wlArgs struct {
	b   []byte
	err error
}

func (a *wlArgs) uint() uint32 {
	if len(a.b) < 4 {
		a.err = errors.New("truncated Wayland message")
		return 0
	}
	v := ne.Uint32(a.b)
	a.b = a.b[4:]
	return v
}

func (a *wlArgs) string() string {
	n := int(a.uint())
	if n == 0 {
		return ""
	}
	padded := (n + 3) &^ 3
	if len(a.b) < padded {
		a.err = errors.New("truncated Wayland message")
		return ""
	}
	s := string(a.b[:n-1])
	a.b = a.b[padded:]
	return s
}

type wlrOutput struct {
	outputID  uint32
	controlID uint32
	name      string
	size      uint32
	failed    bool
}

type wlrGamma struct {
	c          *wlClient
	registryID uint32
	managerID  uint32
	outputs    map[uint32]*wlrOutput // keyed by registry global name
}

func newWlrGamma() (*wlrGamma, error) {
	c, err := wlConnect()
	if err != nil {
		return nil, err
	}
	g := &wlrGamma{c: c, registryID: c.newID(), outputs: map[uint32]*wlrOutput{}}

	type global struct {
		name, version uint32
	}
	var outputs []global
	var manager *global
	c.handlers[g.registryID] = func(op uint16, a *wlArgs) error {
		switch op {
		case wlRegistryGlobal:
			name, iface, version := a.uint(), a.string(), a.uint()
			switch iface {
			case "wl_output":
				if g.managerID == 0 {
					outputs = append(outputs, global{name, version})
				} else if err := g.addOutput(name, version); err != nil {
					return err
				}
			case "zwlr_gamma_control_manager_v1":
				manager = &global{name, version}
			}
		case wlRegistryGlobalRemove:
			g.removeOutput(a.uint())
		}
		return nil
	}

	fail := func(err error) (*wlrGamma, error) {
		c.conn.Close()
		return nil, err
	}
	if err := c.send(wlDisplayID, wlDisplayGetRegistry, wlUint(nil, g.registryID), -1); err != nil {
		return fail(err)
	}
	if err := c.roundtrip(); err != nil {
		return fail(err)
	}
	if manager == nil {
		return fail(errWlrUnsupported)
	}
	if g.managerID, err = c.bind(g.registryID, manager.name, "zwlr_gamma_control_manager_v1", 1); err != nil {
		return fail(err)
	}
	for _, o := range outputs {
		if err := g.addOutput(o.name, o.version); err != nil {
			return fail(err)
		}
	}
	// Wait for every output's gamma_size or failed event.
	if err := c.roundtrip(); err != nil {
		return fail(err)
	}
	usable := 0
	for _, o := range g.outputs {
		if !o.failed && o.size > 0 {
			usable++
			log.Printf("wlr-gamma-control: output %s has %d gamma ramp entries", o.label(), o.size)
		}
	}
	if usable == 0 {
		return fail(errors.New("no output accepted gamma control; is another gamma tool (gammastep, wlsunset, hyprsunset, ...) running?"))
	}
	return g, nil
}

func (g *wlrGamma) addOutput(name, version uint32) error {
	o := &wlrOutput{}
	var err error
	if o.outputID, err = g.c.bind(g.registryID, name, "wl_output", min(version, 4)); err != nil {
		return err
	}
	g.c.handlers[o.outputID] = func(op uint16, a *wlArgs) error {
		if op == wlOutputName {
			o.name = a.string()
		}
		return nil
	}
	o.controlID = g.c.newID()
	if err := g.c.send(g.managerID, wlrGammaManagerGetControl, wlUint(wlUint(nil, o.controlID), o.outputID), -1); err != nil {
		return err
	}
	g.c.handlers[o.controlID] = func(op uint16, a *wlArgs) error {
		switch op {
		case wlrGammaControlGammaSize:
			o.size = a.uint()
		case wlrGammaControlFailed:
			if !o.failed {
				log.Printf("wlr-gamma-control: compositor refused gamma control for output %s (another gamma tool may be running)", o.label())
			}
			o.failed = true
		}
		return nil
	}
	g.outputs[name] = o
	return nil
}

func (g *wlrGamma) removeOutput(name uint32) {
	o, ok := g.outputs[name]
	if !ok {
		return
	}
	delete(g.outputs, name)
	delete(g.c.handlers, o.outputID)
	delete(g.c.handlers, o.controlID)
	g.c.send(o.controlID, wlrGammaControlDestroy, nil, -1)
}

func (o *wlrOutput) label() string {
	if o.name != "" {
		return o.name
	}
	return fmt.Sprintf("#%d", o.outputID)
}

func (g *wlrGamma) Name() string { return "wlr-gamma-control (Wayland)" }

func (g *wlrGamma) Set(gamma float64) error {
	if err := validateGamma(gamma); err != nil {
		return err
	}
	// Pick up hotplugged outputs and failures that arrived since the last call.
	if err := g.c.roundtrip(); err != nil {
		return err
	}
	var sent []*wlrOutput
	for _, o := range g.outputs {
		if o.failed || o.size == 0 {
			continue
		}
		if err := g.setRamp(o, gamma); err != nil {
			return fmt.Errorf("output %s: %w", o.label(), err)
		}
		sent = append(sent, o)
	}
	if err := g.c.roundtrip(); err != nil {
		return err
	}
	applied := 0
	for _, o := range sent {
		if !o.failed {
			applied++
		}
	}
	if applied == 0 {
		return errors.New("no output accepted the gamma ramp")
	}
	return nil
}

// setRamp hands the compositor a file holding the red, green and blue ramps back to back.
func (g *wlrGamma) setRamp(o *wlrOutput, gamma float64) error {
	ramp := buildRamp(int(o.size), gamma)
	channel := unsafe.Slice((*byte)(unsafe.Pointer(&ramp[0])), len(ramp)*2)

	f, err := os.CreateTemp(os.Getenv("XDG_RUNTIME_DIR"), "osu-gamma-ramp-*")
	if err != nil {
		return err
	}
	defer f.Close()
	os.Remove(f.Name())
	for range 3 {
		if _, err := f.Write(channel); err != nil {
			return err
		}
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	return g.c.send(o.controlID, wlrGammaControlSetGamma, nil, int(f.Fd()))
}

func (g *wlrGamma) Close() error {
	// Closing the connection makes the compositor restore the original gamma.
	return g.c.conn.Close()
}
