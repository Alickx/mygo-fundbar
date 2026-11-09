// Command genicon writes the app's images: the menu bar icon, a template
// image of four rising bars, and resources/icon.png, the app's icon.
//
//	go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

func main() {
	if err := trayIcon(); err != nil {
		log.Fatal(err)
	}
	if err := appIcon(); err != nil {
		log.Fatal(err)
	}
}

// trayIcon writes the menu bar icon: black on transparent, so that macOS
// tints it to the menu bar.
func trayIcon() error {
	const size = 36
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := color.NRGBA{0, 0, 0, 255}
	base := 30
	for _, b := range []struct{ x, h int }{{5, 8}, {13, 14}, {21, 20}, {29, 27}} {
		for y := base - b.h; y < base; y++ {
			for x := b.x; x < b.x+4; x++ {
				img.Set(x, y, ink)
			}
		}
		// A rounded top: no corner pixel on the first row.
		img.Set(b.x, base-b.h, color.NRGBA{})
		img.Set(b.x+3, base-b.h, color.NRGBA{})
	}
	return write("resources/tray.png", img)
}

// appIcon writes the app's icon: a rounded square of the two brands' colors
// with four rising bars, drawn at four times the size and averaged down so
// that its edges are smooth.
func appIcon() error {
	const size, ss = 1024, 4
	const n = size * ss
	buf := make([]float64, n*n*4)

	from := color.NRGBA{0x1d, 0x4e, 0xd8, 0xff}
	to := color.NRGBA{0x7c, 0x3a, 0xed, 0xff}

	// The rounded square of the desktop's icons.
	radius := 0.225 * n
	inside := func(x, y float64) bool {
		cx := math.Min(math.Max(x, radius), n-radius)
		cy := math.Min(math.Max(y, radius), n-radius)
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= radius*radius
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !inside(fx, fy) {
				continue
			}
			t := (fx/n + fy/n) / 2
			r := lerp(float64(from.R), float64(to.R), t)
			g := lerp(float64(from.G), float64(to.G), t)
			b := lerp(float64(from.B), float64(to.B), t)
			set(buf, n, x, y, r, g, b, 255)
		}
	}

	// Four bars rising to the right, white, with rounded corners.
	bars := []struct{ x, h float64 }{
		{0.2125, 0.16}, {0.3675, 0.27}, {0.5225, 0.38}, {0.6775, 0.49},
	}
	const barW, baseY, barR = 0.11, 0.75, 0.026
	for _, b := range bars {
		x0, x1 := b.x*n, (b.x+barW)*n
		y0, y1 := (baseY-b.h)*n, baseY*n
		rr := barR * n
		for y := int(y0 - 1); y < int(y1+1); y++ {
			for x := int(x0 - 1); x < int(x1+1); x++ {
				fx, fy := float64(x)+0.5, float64(y)+0.5
				if fx < x0 || fx > x1 || fy < y0 || fy > y1 {
					continue
				}
				// Each corner is a quarter circle of radius rr.
				cx := math.Min(math.Max(fx, x0+rr), x1-rr)
				cy := math.Min(math.Max(fy, y0+rr), y1-rr)
				dx, dy := fx-cx, fy-cy
				if dx*dx+dy*dy > rr*rr {
					continue
				}
				set(buf, n, x, y, 255, 255, 255, 255)
			}
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for dy := 0; dy < ss; dy++ {
				for dx := 0; dx < ss; dx++ {
					i := ((y*ss+dy)*n + (x*ss + dx)) * 4
					r += buf[i]
					g += buf[i+1]
					b += buf[i+2]
					a += buf[i+3]
				}
			}
			k := float64(ss * ss)
			img.Set(x, y, premultiplied(color.NRGBA{
				uint8(clamp(r / k)), uint8(clamp(g / k)),
				uint8(clamp(b / k)), uint8(clamp(a / k)),
			}))
		}
	}
	return write("resources/icon.png", img)
}

// set writes a premultiplied color, as an NRGBA image keeps them, over what
// is there: the bars cover the square's gradient.
func set(buf []float64, n, x, y int, r, g, b, a float64) {
	if x < 0 || y < 0 || x >= n || y >= n {
		return
	}
	i := (y*n + x) * 4
	dst := [4]float64{buf[i], buf[i+1], buf[i+2], buf[i+3]}
	src := [4]float64{r * a / 255, g * a / 255, b * a / 255, a}
	out := [4]float64{
		src[0] + dst[0]*(1-src[3]/255),
		src[1] + dst[1]*(1-src[3]/255),
		src[2] + dst[2]*(1-src[3]/255),
		src[3] + dst[3]*(1-src[3]/255),
	}
	copy(buf[i:i+4], out[:])
}

func premultiplied(c color.NRGBA) color.NRGBA {
	if c.A == 255 {
		return c
	}
	f := float64(c.A) / 255
	return color.NRGBA{
		uint8(float64(c.R) * f),
		uint8(float64(c.G) * f),
		uint8(float64(c.B) * f),
		c.A,
	}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func clamp(v float64) float64 { return math.Min(255, math.Max(0, v)) }

func write(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
