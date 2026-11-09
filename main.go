// Command fundbar shows funds' estimated value today in the menu bar, with
// the funds the user follows in a window.
//
//	go run .
package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fundbar/internal/app"
)

//go:embed resources/tray.png
var trayIcon []byte

func main() {
	mygo.App.SetName("FundBar")
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		a := app.New(dir)

		// A menu bar app: no Dock icon.
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Role: mygo.RoleEditMenu},
			{Role: mygo.RoleWindowMenu},
		}))

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:     "基金估值",
			Width:     1080,
			Height:    780,
			MinWidth:  900,
			MinHeight: 600,
			StateKey:  "main",
			// The view draws the whole window; the window controls float
			// over its top left corner.
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 18, Y: 18},
			Content:              ui.View(a.View),
		})

		tray, err := mygo.NewTray(mygo.TrayOptions{
			ID:             "fundbar",
			Icon:           trayIcon,
			IconIsTemplate: true,
			ToolTip:        "基金估值",
		})
		if err != nil {
			log.Println("tray:", err)
		}

		a.Attach(win, tray)

		quitting := false
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
		win.OnClose(func(e *mygo.CloseEvent) {
			if quitting {
				return
			}
			e.PreventDefault()
			win.Hide()
		})
		if tray != nil {
			// A click opens the panel; the menu waits on a right click.
			tray.OnClick(a.Show)
			tray.OnRightClick(func() {
				tray.SetMenu(a.TrayMenu())
				tray.PopUpMenu()
				tray.SetMenu(nil)
			})
		}

		a.Start()
		win.Show()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
