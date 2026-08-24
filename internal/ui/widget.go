package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/mrv/cursor-usage-widget/internal/cursor"
)

const (
	winW = 270
	winH = 170
	pad  = 3

	closeX = winW - 20
	closeY = pad + 14
	closeR = 11

	gripW = 36
	gripH = 28
	gripX = closeX - closeR - 8 - gripW
	gripY = closeY - gripH/2

	chromeX = gripX - 4
	chromeY = 0
	chromeW = winW - chromeX - 2
	chromeH = gripY + gripH + 8

	contentX = 10
	meterW   = winW - 19

	iconSize  = 18
	titleY    = 8
	remainY   = 28
	closeSlop = 6
)

func WindowSize() (int, int) { return winW, winH }

type Widget struct {
	client *cursor.Client
	poll   time.Duration
	faces  *faces

	iconSrc image.Image
	icon    *ebiten.Image

	mu   sync.RWMutex
	snap cursor.Snapshot

	refreshAt  time.Time
	closeHover bool
	gripHover  bool
	dragging   bool
	dragMX     int
	dragMY     int
	dragWX     int
	dragWY     int
	btn1       bool
	inputReady bool
	boot       int
}

func New(client *cursor.Client, poll time.Duration, icon image.Image) (*Widget, error) {
	f, err := loadFaces()
	if err != nil {
		return nil, err
	}
	w := &Widget{
		client:    client,
		poll:      poll,
		faces:     f,
		iconSrc:   icon,
		refreshAt: time.Now(),
	}
	go w.refresh()
	return w, nil
}

func (w *Widget) Layout(_, _ int) (int, int) { return winW, winH }

func (w *Widget) Update() error {
	cx, cy, rx, ry, btn1 := localCursor()
	glfwJust := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	glfwHeld := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	just := (btn1 && !w.btn1) || glfwJust
	held := btn1 || glfwHeld
	w.btn1 = btn1
	w.closeHover = hitClose(cx, cy, closeSlop)
	w.gripHover = hitChrome(cx, cy) && !w.closeHover
	w.syncCursor()

	w.boot++
	if !w.inputReady {
		if w.boot > 10 && !held {
			w.inputReady = true
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		savePosition()
		return ebiten.Termination
	}
	if w.inputReady && !w.dragging && just && w.closeHover {
		savePosition()
		return ebiten.Termination
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		go w.refresh()
	}

	if w.inputReady {
		w.handleDrag(rx, ry, just, held)
	}
	w.pollIfDue()
	return nil
}

func (w *Widget) syncCursor() {
	switch {
	case w.dragging || w.gripHover:
		ebiten.SetCursorShape(ebiten.CursorShapeMove)
	case w.closeHover:
		ebiten.SetCursorShape(ebiten.CursorShapePointer)
	default:
		ebiten.SetCursorShape(ebiten.CursorShapeDefault)
	}
}

func (w *Widget) handleDrag(rootX, rootY int, just, held bool) {
	if just && w.gripHover && !w.closeHover {
		ox, oy, ok := windowRootPos()
		if !ok {
			return
		}
		w.dragging = true
		w.dragMX, w.dragMY = rootX, rootY
		w.dragWX, w.dragWY = ox, oy
	}
	if !w.dragging {
		return
	}
	if !held {
		w.dragging = false
		savePosition()
		return
	}
	if !moveWindow(w.dragWX+rootX-w.dragMX, w.dragWY+rootY-w.dragMY) {
		wx, wy := ebiten.WindowPosition()
		ebiten.SetWindowPosition(wx+rootX-w.dragMX, wy+rootY-w.dragMY)
		w.dragMX, w.dragMY = rootX, rootY
	}
}

func (w *Widget) pollIfDue() {
	w.mu.Lock()
	due := time.Now().After(w.refreshAt)
	if due {
		w.refreshAt = time.Now().Add(w.poll)
	}
	w.mu.Unlock()
	if due {
		go w.refresh()
	}
}

func (w *Widget) refresh() {
	snap := w.client.Fetch(context.Background())
	w.mu.Lock()
	w.snap = snap
	w.refreshAt = time.Now().Add(w.poll)
	w.mu.Unlock()
}

func (w *Widget) Draw(screen *ebiten.Image) {
	screen.Clear()
	fillRoundRect(screen,
		float32(pad), float32(pad),
		float32(winW-pad*2), float32(winH-pad*2),
		22, rgba(12, 14, 20, 148),
	)

	w.mu.RLock()
	snap := w.snap
	w.mu.RUnlock()

	drawHeader(screen, w.iconImage(), w.faces)
	drawGrip(screen, w.gripHover || w.dragging)
	drawClose(screen, w.closeHover)

	if snap.Err != "" && snap.AutoPct == 0 && snap.APIPct == 0 {
		drawText(screen, wrapErr(snap.Err), w.faces.err, contentX, 72, rgba(255, 170, 170, 230))
		return
	}

	drawRemaining(screen, w.faces, snap)
	drawMeter(screen, w.faces, contentX, 56, meterW, "Auto + Composer", snap.AutoPct)
	drawMeter(screen, w.faces, contentX, 108, meterW, "Other models", snap.APIPct)
}

func (w *Widget) iconImage() *ebiten.Image {
	if w.icon != nil || w.iconSrc == nil {
		return w.icon
	}
	w.icon = ebiten.NewImageFromImage(w.iconSrc)
	return w.icon
}

func drawHeader(dst *ebiten.Image, icon *ebiten.Image, f *faces) {
	x := float64(contentX)
	if icon != nil {
		b := icon.Bounds()
		op := &ebiten.DrawImageOptions{}
		s := float64(iconSize) / float64(b.Dx())
		op.GeoM.Scale(s, s)
		op.GeoM.Translate(x, titleY)
		dst.DrawImage(icon, op)
		x += float64(iconSize) + 6
	}
	drawText(dst, "cursor remaining", f.meta, x, titleY+2, rgba(230, 235, 245, 210))
}

func drawRemaining(dst *ebiten.Image, f *faces, snap cursor.Snapshot) {
	amt := formatMoney(snap.RemainingUSD)
	drawText(dst, amt, f.remain, contentX, remainY, usageColor(snap.IncludedPct))
	aw, _ := text.Measure(amt, f.remain, 0)

	reset := formatResets(snap.CycleEnd)
	if reset == "" {
		return
	}
	rw, _ := text.Measure(reset, f.meta, 0)
	rx := float64(gripX-8) - rw
	if rx < contentX+aw+10 {
		return
	}
	drawText(dst, reset, f.meta, rx, remainY+4, rgba(230, 235, 245, 150))
}

func formatMoney(v float64) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatResets(end time.Time) string {
	if end.IsZero() {
		return ""
	}
	d := time.Until(end)
	if d <= 0 {
		return "resets soon"
	}
	hours := d.Hours()
	if hours < 24 {
		h := int(math.Ceil(hours))
		if h < 1 {
			return "resets soon"
		}
		return fmt.Sprintf("resets in %dh", h)
	}
	return fmt.Sprintf("resets in %dd", int(hours/24))
}

func drawClose(dst *ebiten.Image, hover bool) {
	cx, cy := float32(closeX), float32(closeY)
	bg := rgba(220, 56, 70, 240)
	if hover {
		bg = rgba(255, 72, 86, 255)
	}
	vector.FillCircle(dst, cx, cy, float32(closeR), rgba(0, 0, 0, 80), true)
	vector.FillCircle(dst, cx, cy, float32(closeR)-1, bg, true)
	const s float32 = 4.2
	fg := rgba(255, 255, 255, 255)
	vector.StrokeLine(dst, cx-s, cy-s, cx+s, cy+s, 2.4, fg, true)
	vector.StrokeLine(dst, cx+s, cy-s, cx-s, cy+s, 2.4, fg, true)
}

func hitClose(x, y int, extra float64) bool {
	dx := float64(x - closeX)
	dy := float64(y - closeY)
	r := float64(closeR) + extra
	return dx*dx+dy*dy <= r*r
}

func drawGrip(dst *ebiten.Image, hover bool) {
	bg := rgba(40, 48, 62, 210)
	fg := rgba(210, 218, 230, 220)
	if hover {
		bg = rgba(58, 70, 92, 235)
		fg = rgba(255, 255, 255, 255)
	}
	fillRoundRect(dst, float32(gripX), float32(gripY), float32(gripW), float32(gripH), 8, bg)
	cx := float32(gripX) + float32(gripW)/2
	cy := float32(gripY) + float32(gripH)/2
	const arm float32 = 7
	const head float32 = 3.2
	const th float32 = 1.8
	vector.StrokeLine(dst, cx, cy-arm, cx, cy+arm, th, fg, true)
	vector.StrokeLine(dst, cx-arm, cy, cx+arm, cy, th, fg, true)
	vector.StrokeLine(dst, cx, cy-arm, cx-head, cy-arm+head, th, fg, true)
	vector.StrokeLine(dst, cx, cy-arm, cx+head, cy-arm+head, th, fg, true)
	vector.StrokeLine(dst, cx, cy+arm, cx-head, cy+arm-head, th, fg, true)
	vector.StrokeLine(dst, cx, cy+arm, cx+head, cy+arm-head, th, fg, true)
	vector.StrokeLine(dst, cx-arm, cy, cx-arm+head, cy-head, th, fg, true)
	vector.StrokeLine(dst, cx-arm, cy, cx-arm+head, cy+head, th, fg, true)
	vector.StrokeLine(dst, cx+arm, cy, cx+arm-head, cy-head, th, fg, true)
	vector.StrokeLine(dst, cx+arm, cy, cx+arm-head, cy+head, th, fg, true)
}

func hitChrome(x, y int) bool {
	return x >= chromeX && x < chromeX+chromeW &&
		y >= chromeY && y < chromeY+chromeH
}

func drawMeter(dst *ebiten.Image, f *faces, x, y, width int, label string, pct float64) {
	drawText(dst, label, f.label, float64(x), float64(y), rgba(230, 235, 245, 220))
	val := fmt.Sprintf("%.0f%%", clamp(pct, 0, 999))
	vw, _ := text.Measure(val, f.pct, 0)
	drawText(dst, val, f.pct, float64(x+width)-vw, float64(y)-4, usageColor(pct))

	bx, by := float32(x), float32(y+28)
	bw, bh := float32(width), float32(10)
	fillRoundRect(dst, bx, by, bw, bh, 6, rgba(255, 255, 255, 22))
	if fillW := float32(clamp(pct, 0, 100) / 100 * float64(bw)); fillW > 2 {
		fillRoundRect(dst, bx, by, fillW, bh, 6, usageColor(pct))
	}
}

func drawText(dst *ebiten.Image, s string, face *text.GoTextFace, x, y float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face, op)
}

func usageColor(pct float64) color.Color {
	switch {
	case pct >= 100:
		return rgba(255, 92, 122, 240)
	case pct >= 80:
		return rgba(255, 138, 76, 240)
	case pct >= 50:
		return rgba(245, 197, 66, 240)
	default:
		return rgba(80, 220, 160, 240)
	}
}

func wrapErr(s string) string {
	if len(s) > 36 {
		return s[:34] + "…"
	}
	return s
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}

func rgba(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: a}
}

var roundMaskCache = map[[3]int]*ebiten.Image{}

func roundMask(w, h, r float32) *ebiten.Image {
	key := [3]int{int(math.Ceil(float64(w))), int(math.Ceil(float64(h))), int(math.Ceil(float64(r * 10)))}
	if img, ok := roundMaskCache[key]; ok {
		return img
	}
	img := ebiten.NewImage(key[0], key[1])
	img.Fill(color.Transparent)
	if r*2 > w {
		r = w / 2
	}
	if r*2 > h {
		r = h / 2
	}
	white := color.RGBA{255, 255, 255, 255}
	vector.FillRect(img, r, 0, w-2*r, h, white, true)
	vector.FillRect(img, 0, r, w, h-2*r, white, true)
	vector.FillCircle(img, r, r, r, white, true)
	vector.FillCircle(img, w-r, r, r, white, true)
	vector.FillCircle(img, r, h-r, r, white, true)
	vector.FillCircle(img, w-r, h-r, r, white, true)
	roundMaskCache[key] = img
	return img
}

func fillRoundRect(dst *ebiten.Image, x, y, w, h, r float32, clr color.Color) {
	if w < 1 || h < 1 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	cr, cg, cb, ca := clr.RGBA()
	op.ColorScale.Scale(
		float32(cr)/0xffff,
		float32(cg)/0xffff,
		float32(cb)/0xffff,
		float32(ca)/0xffff,
	)
	dst.DrawImage(roundMask(w, h, r), op)
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
		dir = filepath.Join(dir, ".config")
	}
	return filepath.Join(dir, "cursor-usage-widget", "state.json")
}

func savePosition() {
	x, y := ebiten.WindowPosition()
	rx, ry, ok := windowRootPos()
	_ = os.MkdirAll(filepath.Dir(statePath()), 0o755)
	st := map[string]int{"x": x, "y": y}
	if ok {
		st["rootX"] = rx
		st["rootY"] = ry
	}
	b, _ := json.Marshal(st)
	_ = os.WriteFile(statePath(), b, 0o644)
}

func RestorePosition() {
	b, err := os.ReadFile(statePath())
	if err != nil {
		sw, _ := ebiten.Monitor().Size()
		ebiten.SetWindowPosition(sw-winW-24, 48)
		return
	}
	var p struct {
		X, Y        int
		RootX, RootY *int
	}
	if json.Unmarshal(b, &p) != nil {
		return
	}
	if p.RootX != nil && p.RootY != nil {
		savedRoot.x, savedRoot.y, savedRoot.ok = *p.RootX, *p.RootY, true
		return
	}
	sw, _ := ebiten.Monitor().Size()
	ebiten.SetWindowPosition(sw-winW-24, 48)
}
