//go:build windows

package gui

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/lxn/walk"
)

func (a *App) loadClientIcons() error {
	idle, idleErr := a.loadClientIconFile("client.ico")
	if idleErr != nil {
		idle, idleErr = newGeneratedClientIcon(false)
	}
	connected, connectedErr := a.loadClientIconFile("client-connected.ico")
	if connectedErr != nil {
		connected, connectedErr = newGeneratedClientIcon(true)
	}
	a.iconIdle = idle
	a.iconConnected = connected
	if idleErr != nil {
		return idleErr
	}
	return connectedErr
}

func (a *App) loadClientIconFile(name string) (*walk.Icon, error) {
	for _, path := range a.clientIconCandidates(name) {
		if _, err := os.Stat(path); err == nil {
			return walk.NewIconFromFileWithSize(path, walk.Size{Width: 32, Height: 32})
		}
	}
	return nil, os.ErrNotExist
}

func (a *App) clientIconCandidates(name string) []string {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "assets", name),
			filepath.Join(exeDir, "assets", name),
		)
	}
	candidates = append(candidates,
		filepath.Join(a.workspace, "assets", name),
		filepath.Join(a.workspace, "client", "assets", name),
	)
	return candidates
}

func (a *App) iconForState() *walk.Icon {
	if a.isRunning() && a.iconConnected != nil {
		return a.iconConnected
	}
	return a.iconIdle
}

func (a *App) refreshClientIcon() {
	a.runOnUI(func() { a.refreshClientIconNow() })
}

func (a *App) refreshClientIconNow() {
	running := a.isRunning()
	if a.currentIconKnown && a.currentIconConnected == running {
		return
	}
	icon := a.iconForState()
	if icon == nil {
		return
	}
	if a.mw != nil {
		_ = a.mw.SetIcon(icon)
	}
	if a.ni != nil {
		_ = a.ni.SetIcon(icon)
	}
	a.currentIconKnown = true
	a.currentIconConnected = running
}

func (a *App) disposeClientIcons() {
	if a.iconIdle != nil {
		a.iconIdle.Dispose()
		a.iconIdle = nil
	}
	if a.iconConnected != nil {
		a.iconConnected.Dispose()
		a.iconConnected = nil
	}
}

func newGeneratedClientIcon(connected bool) (*walk.Icon, error) {
	return walk.NewIconFromImage(renderClientIcon(64, connected))
}

func renderClientIcon(size int, connected bool) image.Image {
	ss := 4
	n := size * ss
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	s := float64(n)

	margin := 0.09 * s
	drawRoundedIconRect(img, margin, margin, s-margin, s-margin, 0.23*s, func(_ float64, y float64) color.RGBA {
		return mixColor(color.RGBA{16, 42, 67, 255}, color.RGBA{13, 143, 135, 255}, y/s)
	})
	drawIconStroke(img, 0.31*s, 0.71*s, 0.50*s, 0.27*s, 0.095*s, color.RGBA{255, 255, 255, 245})
	drawIconStroke(img, 0.50*s, 0.27*s, 0.69*s, 0.71*s, 0.095*s, color.RGBA{255, 255, 255, 245})
	drawIconStroke(img, 0.40*s, 0.55*s, 0.60*s, 0.55*s, 0.075*s, color.RGBA{217, 255, 246, 245})
	if connected {
		drawIconCircle(img, 0.76*s, 0.76*s, 0.17*s, color.RGBA{255, 255, 255, 255})
		drawIconCircle(img, 0.76*s, 0.76*s, 0.115*s, color.RGBA{34, 197, 94, 255})
	}
	return downsampleIcon(img, size, ss)
}

func drawRoundedIconRect(img *image.RGBA, x0, y0, x1, y1, radius float64, fill func(x, y float64) color.RGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		py := float64(y) + 0.5
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			px := float64(x) + 0.5
			cx := clampFloat(px, x0+radius, x1-radius)
			cy := clampFloat(py, y0+radius, y1-radius)
			dx := px - cx
			dy := py - cy
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, fill(px, py))
			}
		}
	}
}

func drawIconStroke(img *image.RGBA, x1, y1, x2, y2, width float64, c color.RGBA) {
	r := width / 2
	minX := int(math.Floor(math.Min(x1, x2) - r - 1))
	maxX := int(math.Ceil(math.Max(x1, x2) + r + 1))
	minY := int(math.Floor(math.Min(y1, y2) - r - 1))
	maxY := int(math.Ceil(math.Max(y1, y2) + r + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			if distanceToSegment(float64(x)+0.5, float64(y)+0.5, x1, y1, x2, y2) <= r {
				blendIconPixel(img, x, y, c)
			}
		}
	}
	drawIconCircle(img, x1, y1, r, c)
	drawIconCircle(img, x2, y2, r, c)
}

func drawIconCircle(img *image.RGBA, cx, cy, radius float64, c color.RGBA) {
	minX := int(math.Floor(cx - radius - 1))
	maxX := int(math.Ceil(cx + radius + 1))
	minY := int(math.Floor(cy - radius - 1))
	maxY := int(math.Ceil(cy + radius + 1))
	r2 := radius * radius
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if dx*dx+dy*dy <= r2 {
				blendIconPixel(img, x, y, c)
			}
		}
	}
}

func blendIconPixel(img *image.RGBA, x, y int, src color.RGBA) {
	dst := img.RGBAAt(x, y)
	sa := float64(src.A) / 255
	da := float64(dst.A) / 255
	outA := sa + da*(1-sa)
	if outA == 0 {
		img.SetRGBA(x, y, color.RGBA{})
		return
	}
	r := (float64(src.R)*sa + float64(dst.R)*da*(1-sa)) / outA
	g := (float64(src.G)*sa + float64(dst.G)*da*(1-sa)) / outA
	b := (float64(src.B)*sa + float64(dst.B)*da*(1-sa)) / outA
	img.SetRGBA(x, y, color.RGBA{uint8(r + 0.5), uint8(g + 0.5), uint8(b + 0.5), uint8(outA*255 + 0.5)})
}

func downsampleIcon(src *image.RGBA, size, scale int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	area := uint32(scale * scale)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for yy := 0; yy < scale; yy++ {
				for xx := 0; xx < scale; xx++ {
					c := src.RGBAAt(x*scale+xx, y*scale+yy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / area), uint8(g / area), uint8(b / area), uint8(a / area)})
		}
	}
	return dst
}

func mixColor(a, b color.RGBA, t float64) color.RGBA {
	t = clampFloat(t, 0, 1)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t + 0.5),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t + 0.5),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t + 0.5),
		A: 255,
	}
}

func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := clampFloat(((px-x1)*dx+(py-y1)*dy)/l2, 0, 1)
	cx := x1 + t*dx
	cy := y1 + t*dy
	return math.Hypot(px-cx, py-cy)
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
