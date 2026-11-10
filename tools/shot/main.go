// Command shot draws the window to a PNG without opening it, to look at the
// layout while working on it.
//
//	go run ./tools/shot /tmp/fundbar.png
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/app"
	"fundbar/internal/source"
)

func main() {
	out := "/tmp/fundbar.png"
	dark := false
	width, height := 1080, 840
	if v := os.Getenv("FUNDBAR_W"); v != "" {
		fmt.Sscan(v, &width)
	}
	if v := os.Getenv("FUNDBAR_H"); v != "" {
		fmt.Sscan(v, &height)
	}
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if len(os.Args) > 2 && os.Args[2] == "dark" {
		dark = true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Join(home, "Library", "Application Support", "FundBar")
	if d := os.Getenv("FUNDBAR_DIR"); d != "" {
		dir = d
	}
	a := app.New(dir)
	a.Load()
	if len(os.Args) > 3 {
		switch os.Args[3] {
		case "search":
			a.PreviewSearch("白酒", []source.Hit{
				{Code: "161725", Name: "招商中证白酒指数(LOF)A", Type: "指数型-股票"},
				{Code: "012414", Name: "招商中证白酒指数(LOF)C", Type: "指数型-股票"},
				{Code: "0124142", Name: "鹏华酒指数A", Type: "指数型-股票"},
			})
		case "settings":
			tester := ui.NewTesterWith(a.PreviewSettings, 480, 430,
				ui.TesterOptions{Dark: dark})
			tester.Frame()
			img := tester.Image()
			f, err := os.Create(out)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				log.Fatal(err)
			}
			return
		case "edit":
			a.PreviewEdit(true)
			a.PreviewLogin(true)
		case "empty":
			a = app.New(filepath.Join(os.TempDir(), "fundbar-empty"))
			a.Load()
		case "group":
			a.PreviewGroup(os.Getenv("FUNDBAR_GROUP"))
		case "groupnew":
			a.PreviewGroupEdit()
		case "panel":
			tester := ui.NewTesterWith(a.Panel, 320, 560,
				ui.TesterOptions{Dark: dark})
			tester.Frame()
			// The preview draws no background of its own where the system's
			// material shows; a grey stands in for what lies behind the
			// menu bar, which is what the numbers have to read on.
			tester.SetVibrancy(true)
			img := over(trim(tester.Image(), 14), dark)
			f, err := os.Create(out)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	if len(os.Args) > 4 {
		a.ShowFund(os.Args[4])
	}

	tester := ui.NewTesterWith(a.View, width, height, ui.TesterOptions{Dark: dark})
	tester.Frame()
	img := tester.Image()
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// trim takes the margin below a preview away, keeping pad rows of it.
func trim(img *image.RGBA, pad int) *image.RGBA {
	b := img.Bounds()
	height := b.Max.Y
	for y := b.Max.Y - 1; y >= b.Min.Y && height == b.Max.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				height = min(y+1+pad, b.Max.Y)
				break
			}
		}
	}
	return img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, height)).(*image.RGBA)
}

// over draws img on the grey the menu bar's material leaves under a
// preview: a light one in the dark appearance, a bright one in the light.
func over(img *image.RGBA, dark bool) *image.RGBA {
	b := img.Bounds()
	top, bottom := color.RGBA{0x3c, 0x3c, 0x44, 0xff}, color.RGBA{0x74, 0x74, 0x7c, 0xff}
	if !dark {
		top, bottom = color.RGBA{0xda, 0xda, 0xe2, 0xff}, color.RGBA{0xf4, 0xf4, 0xf8, 0xff}
	}
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		t := float32(y-b.Min.Y) / float32(max(b.Dy()-1, 1))
		c := color.RGBA{
			R: uint8(float32(top.R)*(1-t) + float32(bottom.R)*t),
			G: uint8(float32(top.G)*(1-t) + float32(bottom.G)*t),
			B: uint8(float32(top.B)*(1-t) + float32(bottom.B)*t),
			A: 0xff,
		}
		draw.Draw(out, image.Rect(b.Min.X, y, b.Max.X, y+1),
			&image.Uniform{c}, image.Point{}, draw.Src)
	}
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}
