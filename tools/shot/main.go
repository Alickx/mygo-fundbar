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
	"strings"

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
		case "batch":
			query := os.Getenv("FUNDBAR_BATCH")
			if query == "" {
				query = "007760, 161725"
			}
			a.PreviewBatch(query)
		case "settings":
			if os.Getenv("FUNDBAR_ALERTS") != "" {
				a.PreviewAlerts(true, 2, true)
			}
			if os.Getenv("FUNDBAR_ALERT_DENIED") != "" {
				a.PreviewAlertDenied()
			}
			if v := os.Getenv("FUNDBAR_TRAY"); v != "" {
				a.PreviewTrayText(v)
			}
			if os.Getenv("FUNDBAR_SHORTCUT") != "" {
				a.PreviewGlobalShortcut(true)
			}
			if os.Getenv("FUNDBAR_INDICES") == "off" {
				a.PreviewIndices(false)
			}
			tester := ui.NewTesterWith(a.PreviewSettings, 480, 760,
				ui.TesterOptions{Dark: dark})
			tester.Frame()
			// FUNDBAR_SCROLL brings a section below the fold into the shot, as
			// a wheel would: the 数据 section needs it at the window's height.
			if v := os.Getenv("FUNDBAR_SCROLL"); v != "" {
				var dy float32
				fmt.Sscan(v, &dy)
				tester.Scroll(240, 600, 0, dy)
				tester.Frame()
			}
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
		case "targets":
			if len(os.Args) > 4 {
				a.PreviewTargets(os.Args[4], 20, 8)
			}
		case "targets-edit":
			a.PreviewEdit(true)
			if len(os.Args) > 4 {
				a.PreviewTargets(os.Args[4], 20, 8)
			}
		case "portfolio":
			a.PreviewPortfolio(true)
		case "compare":
			// The chart with other funds drawn beside the chosen one.
			period := 2
			if v := os.Getenv("FUNDBAR_PERIOD"); v != "" {
				fmt.Sscan(v, &period)
			}
			a.PreviewPeriod(period)
			codes := strings.Split(os.Getenv("FUNDBAR_COMPARE"), ",")
			if len(codes) == 1 && strings.TrimSpace(codes[0]) == "" {
				codes = []string{"008270", "011555", "028678"}
			}
			for i := range codes {
				codes[i] = strings.TrimSpace(codes[i])
			}
			a.PreviewCompare(codes...)
		case "share":
			// The share picture writes itself; FUNDBAR_SHARE_MONEY shows the
			// variant with the user's amounts.
			img := a.SharePreview(os.Getenv("FUNDBAR_SHARE_MONEY") != "")
			f, err := os.Create(out)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, img); err != nil {
				log.Fatal(err)
			}
			return
		case "risk":
			// Select the window whose risk line the shot is about; 近1年
			// unless FUNDBAR_PERIOD names another index.
			period := 3
			if v := os.Getenv("FUNDBAR_PERIOD"); v != "" {
				fmt.Sscan(v, &period)
			}
			a.PreviewPeriod(period)
		case "recovery":
			// The chart card's recovery view, over 近1年 unless
			// FUNDBAR_PERIOD names another index.
			period := 3
			if v := os.Getenv("FUNDBAR_PERIOD"); v != "" {
				fmt.Sscan(v, &period)
			}
			a.PreviewPeriod(period)
			a.PreviewNavView(2)
		case "ledger", "ledger-add", "ledger-edit", "ledger-error":
			if len(os.Args) > 4 {
				a.PreviewLedger(os.Args[4], os.Args[3])
			}
		case "update":
			a.PreviewUpdate("0.4.1", os.Getenv("FUNDBAR_NOTES"),
				os.Getenv("FUNDBAR_STEP"), os.Getenv("FUNDBAR_NOTES_OPEN") != "")
		case "panel":
			a.PreviewMarketIndices()
			if os.Getenv("FUNDBAR_INDICES") == "off" {
				a.PreviewIndices(false)
			}
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
