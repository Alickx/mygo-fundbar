// Command genicon writes the menu bar icon, a template image of four rising
// bars.
//
//	go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const size = 36

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := color.NRGBA{0, 0, 0, 255}

	// Four bars, each taller than the one before, on a common base line.
	bars := []struct{ x, h int }{{5, 8}, {13, 14}, {21, 20}, {29, 27}}
	base := 30
	for _, b := range bars {
		for y := base - b.h; y < base; y++ {
			for x := b.x; x < b.x+4; x++ {
				img.Set(x, y, ink)
			}
		}
		// Rounded top: a single pixel at each corner.
		img.Set(b.x, base-b.h, color.NRGBA{})
		img.Set(b.x+3, base-b.h, color.NRGBA{})
	}

	f, err := os.Create("resources/tray.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
