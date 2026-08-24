package ui

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/shape"
	"github.com/jezek/xgb/xproto"
)

func localCursor() (x, y, rootX, rootY int, btn1 bool) {
	if x, y, rootX, rootY, btn1, ok := x11LocalCursor(); ok {
		return x, y, rootX, rootY, btn1
	}
	x, y = ebiten.CursorPosition()
	wx, wy := ebiten.WindowPosition()
	return x, y, wx + x, wy + y, ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
}

type x11ptr struct {
	conn    *xgb.Conn
	root    xproto.Window
	win     xproto.Window
	pidAtm  xproto.Atom
	listAtm xproto.Atom
	walked  bool
	shaped  bool
}

var (
	x11Once   sync.Once
	x11State  *x11ptr
	savedRoot struct {
		x, y int
		ok   bool
	}
)

func x11LocalCursor() (int, int, int, int, bool, bool) {
	s := x11Session()
	if s == nil {
		return 0, 0, 0, 0, false, false
	}
	s.applyChromeShape()
	reply, err := xproto.QueryPointer(s.conn, s.root).Reply()
	if err != nil || reply == nil {
		return 0, 0, 0, 0, false, false
	}
	ox, oy, ok := s.clientOrigin()
	if !ok {
		return 0, 0, 0, 0, false, false
	}
	scale := deviceScale()
	rx, ry := int(reply.RootX), int(reply.RootY)
	return int(float64(rx-ox) / scale), int(float64(ry-oy) / scale),
		rx, ry, reply.Mask&xproto.KeyButMaskButton1 != 0, true
}

func deviceScale() float64 {
	if m := ebiten.Monitor(); m != nil {
		if s := m.DeviceScaleFactor(); s > 0 {
			return s
		}
	}
	return 1
}

func x11Session() *x11ptr {
	x11Once.Do(func() {
		conn, err := xgb.NewConn()
		if err != nil {
			warnPointer(err)
			return
		}
		setup := xproto.Setup(conn)
		if setup == nil || len(setup.Roots) == 0 {
			conn.Close()
			warnPointer(fmt.Errorf("no X11 screens"))
			return
		}
		if err := shape.Init(conn); err != nil {
			conn.Close()
			warnPointer(err)
			return
		}
		s := &x11ptr{conn: conn, root: setup.DefaultScreen(conn).Root}
		s.pidAtm = internAtom(conn, "_NET_WM_PID")
		s.listAtm = internAtom(conn, "_NET_CLIENT_LIST")
		x11State = s
	})
	return x11State
}

func (s *x11ptr) applyChromeShape() {
	if !s.ensureWin() {
		return
	}
	if savedRoot.ok {
		s.moveToplevel(savedRoot.x, savedRoot.y)
		savedRoot.ok = false
	}
	if s.shaped {
		return
	}
	scale := deviceScale()
	x := int16(math.Round(float64(chromeX) * scale))
	y := int16(math.Round(float64(chromeY) * scale))
	w := uint16(math.Round(float64(chromeW) * scale))
	h := uint16(math.Round(float64(chromeH) * scale))
	if w == 0 || h == 0 {
		return
	}
	shape.Rectangles(s.conn, shape.SoSet, shape.SkInput, 0, s.win, 0, 0, []xproto.Rectangle{{
		X: x, Y: y, Width: w, Height: h,
	}})
	s.shaped = true
}

func (s *x11ptr) forgetWin() {
	s.win = 0
	s.shaped = false
	s.walked = false
}

func (s *x11ptr) ensureWin() bool {
	if s.win != 0 {
		if s.alive(s.win) {
			return true
		}
		s.forgetWin()
	}
	pid := uint32(os.Getpid())
	var classHit xproto.Window
	for _, w := range s.clientWindows() {
		if s.windowPID(w) == pid {
			s.win = w
			return true
		}
		if classHit == 0 && s.hasOurClass(w) {
			classHit = w
		}
	}
	if classHit == 0 {
		return false
	}
	s.win = classHit
	return true
}

func (s *x11ptr) alive(w xproto.Window) bool {
	g, err := xproto.GetGeometry(s.conn, xproto.Drawable(w)).Reply()
	return err == nil && g != nil
}

func (s *x11ptr) clientOrigin() (int, int, bool) {
	if !s.ensureWin() {
		return 0, 0, false
	}
	tr, err := xproto.TranslateCoordinates(s.conn, s.win, s.root, 0, 0).Reply()
	if err != nil || tr == nil {
		s.forgetWin()
		return 0, 0, false
	}
	return int(tr.DstX), int(tr.DstY), true
}

func (s *x11ptr) toplevel() xproto.Window {
	w := s.win
	for i := 0; i < 16; i++ {
		t, err := xproto.QueryTree(s.conn, w).Reply()
		if err != nil || t == nil || t.Parent == 0 || t.Parent == s.root {
			return w
		}
		w = t.Parent
	}
	return w
}

func (s *x11ptr) toplevelOrigin() (int, int, bool) {
	if !s.ensureWin() {
		return 0, 0, false
	}
	top := s.toplevel()
	tr, err := xproto.TranslateCoordinates(s.conn, top, s.root, 0, 0).Reply()
	if err != nil || tr == nil {
		s.forgetWin()
		return 0, 0, false
	}
	return int(tr.DstX), int(tr.DstY), true
}

func (s *x11ptr) moveToplevel(x, y int) {
	if !s.ensureWin() {
		return
	}
	top := s.toplevel()
	_ = xproto.ConfigureWindow(s.conn, top, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{
		uint32(int32(x)),
		uint32(int32(y)),
	})
}

func windowRootPos() (int, int, bool) {
	s := x11Session()
	if s == nil {
		return 0, 0, false
	}
	return s.toplevelOrigin()
}

func moveWindow(x, y int) bool {
	s := x11Session()
	if s == nil {
		return false
	}
	s.moveToplevel(x, y)
	return true
}

func (s *x11ptr) clientWindows() []xproto.Window {
	if s.listAtm != 0 {
		prop, err := xproto.GetProperty(s.conn, false, s.root, s.listAtm, xproto.GetPropertyTypeAny, 0, 4096).Reply()
		if err == nil && prop != nil && len(prop.Value) >= 4 {
			return windowsFrom(prop.Value)
		}
	}
	if s.walked {
		return nil
	}
	s.walked = true
	return s.walk(s.root, 0)
}

func (s *x11ptr) walk(w xproto.Window, depth int) []xproto.Window {
	if depth > 24 {
		return nil
	}
	tree, err := xproto.QueryTree(s.conn, w).Reply()
	if err != nil || tree == nil {
		return nil
	}
	out := make([]xproto.Window, 0, len(tree.Children))
	for _, c := range tree.Children {
		out = append(out, c)
		out = append(out, s.walk(c, depth+1)...)
	}
	return out
}

func (s *x11ptr) windowPID(w xproto.Window) uint32 {
	if s.pidAtm == 0 {
		return 0
	}
	prop, err := xproto.GetProperty(s.conn, false, w, s.pidAtm, xproto.GetPropertyTypeAny, 0, 1).Reply()
	if err != nil || prop == nil || len(prop.Value) < 4 {
		return 0
	}
	return xgb.Get32(prop.Value)
}

func (s *x11ptr) hasOurClass(w xproto.Window) bool {
	prop, err := xproto.GetProperty(s.conn, false, w, xproto.AtomWmClass, xproto.GetPropertyTypeAny, 0, 256).Reply()
	if err != nil || prop == nil {
		return false
	}
	return strings.Contains(string(prop.Value), "cursor-remaining")
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	r, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
	if err != nil || r == nil {
		return 0
	}
	return r.Atom
}

func windowsFrom(b []byte) []xproto.Window {
	out := make([]xproto.Window, 0, len(b)/4)
	for i := 0; i+4 <= len(b); i += 4 {
		out = append(out, xproto.Window(xgb.Get32(b[i:])))
	}
	return out
}

func warnPointer(err error) {
	msg := "cursor-usage-widget: X11 pointer query failed; close/drag need X11 (or XWayland) while click-through is on"
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		fmt.Fprintf(os.Stderr, "%s (Wayland session): %v\n", msg, err)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
}
