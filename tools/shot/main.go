// Command shot draws the window to a PNG without opening it, to look at the
// layout while working on it.
//
//	go run ./tools/shot /tmp/fundbar.png
package main

import (
	"fmt"
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
