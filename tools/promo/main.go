// Command promo renders the current app views and composes README artwork.
// Run from the repository root: go run ./tools/promo
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/app"
	"fundbar/internal/store"
)

const (
	posterW = 1600
	posterH = 1120
	scale   = 2
)

type snapshotInfo struct {
	Code   string    `json:"code"`
	At     time.Time `json:"cached_at"`
	SHA256 string    `json:"sha256"`
}

type manifest struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Version     string            `json:"version"`
	Renderer    string            `json:"renderer"`
	Holdings    string            `json:"holdings"`
	Sources     map[string]string `json:"source_sha256"`
	Snapshots   []snapshotInfo    `json:"snapshots"`
	Images      map[string]string `json:"image_sha256"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := flag.String("data", filepath.Join(home, "Library", "Application Support", "FundBar"), "directory containing the app's cache folder (read only)")
	out := flag.String("out", "docs/promo", "output directory")
	codes := flag.String("codes", "011555,007760,008270,028678", "one to four cached fund codes; the last is selected")
	flag.Parse()

	// Construct independent sample holdings. Never load or change funds.json
	// from the user's app, and never start its refresh/update/network loop.
	tmp, err := os.MkdirTemp("", "fundbar-promo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	snapshots, err := prepare(*data, tmp, strings.Split(*codes, ","))
	if err != nil {
		return err
	}
	iconBytes, err := os.ReadFile("resources/icon.png")
	if err != nil {
		return fmt.Errorf("run from the repository root: %w", err)
	}
	icon, err := ui.DecodeImage(iconBytes)
	if err != nil {
		return err
	}
	meta := manifest{
		GeneratedAt: time.Now().UTC(),
		Renderer:    "Current app.View and app.Panel, MyGo offscreen at 2x; panel material simulated; no image generation service",
		Holdings:    "Demo only: 1000 shares per fund, cost equal to its latest cached published NAV; real user holdings are not read",
		Snapshots:   snapshots,
		Sources:     map[string]string{},
		Images:      map[string]string{},
	}
	config, err := os.ReadFile("mygo.json")
	if err != nil {
		return err
	}
	var version struct{ Version string }
	if err := json.Unmarshal(config, &version); err != nil {
		return err
	}
	meta.Version = version.Version
	files := []string{"go.mod", "go.sum", "mygo.json", "resources/icon.png", "tools/promo/main.go"}
	err = filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		meta.Sources[path] = hash(body)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for _, dark := range []bool{false, true} {
		appearance := "light"
		if dark {
			appearance = "dark"
		}
		a := app.New(tmp)
		a.Load()
		main := render(a.View, 1080, 840, dark, false)
		panel := panelMaterial(trim(render(a.Panel, 320, 560, dark, true)), dark)
		hero := poster(main, panel, icon, dark)
		for _, output := range []struct {
			name string
			img  image.Image
		}{
			{"main-" + appearance + ".png", main},
			{"panel-" + appearance + ".png", panel},
			{"hero-" + appearance + ".png", hero},
		} {
			path := filepath.Join(*out, output.name)
			if err := savePNG(path, output.img); err != nil {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			meta.Images[output.name] = hash(body)
			fmt.Printf("%s  %d × %d  %.0f KB\n", path, output.img.Bounds().Dx(), output.img.Bounds().Dy(), float64(len(body))/1024)
		}
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, "manifest.json"), append(body, '\n'), 0o644)
}

func prepare(src, dst string, codes []string) ([]snapshotInfo, error) {
	if len(codes) < 1 || len(codes) > 4 {
		return nil, fmt.Errorf("choose one to four fund codes for the poster layout")
	}
	valid := regexp.MustCompile(`^\d{6}$`)
	seen := map[string]bool{}
	data := &store.Data{Period: 1, SidebarWidth: 348, Groups: []store.Group{{ID: "watch", Name: "关注"}}}
	var snapshots []snapshotInfo
	if err := os.MkdirAll(filepath.Join(dst, "cache"), 0o755); err != nil {
		return nil, err
	}
	for i, raw := range codes {
		code := strings.TrimSpace(raw)
		if !valid.MatchString(code) || seen[code] {
			return nil, fmt.Errorf("invalid or repeated fund code %q", code)
		}
		seen[code] = true
		body, err := os.ReadFile(filepath.Join(src, "cache", code+".json"))
		if err != nil {
			return nil, fmt.Errorf("cached fund %s unavailable; refresh it in FundBar first, or choose available codes with -codes: %w", code, err)
		}
		var snap store.Snapshot
		if err := json.Unmarshal(body, &snap); err != nil {
			return nil, fmt.Errorf("cached fund %s: %w", code, err)
		}
		if snap.Fund == nil || snap.Fund.Code != code || len(snap.Fund.Navs) == 0 {
			return nil, fmt.Errorf("cached fund %s has no matching fund / NAV data", code)
		}
		last := snap.Fund.Navs[len(snap.Fund.Navs)-1]
		if last.Unit <= 0 {
			return nil, fmt.Errorf("cached fund %s has no positive latest NAV", code)
		}
		if err := os.WriteFile(filepath.Join(dst, "cache", code+".json"), body, 0o600); err != nil {
			return nil, err
		}
		groups := []string{""}
		if i < 2 {
			groups = append(groups, "watch")
		}
		data.Funds = append(data.Funds, store.Entry{Code: code, Shares: 1000, Cost: last.Unit, Groups: groups})
		data.Selected = code
		snapshots = append(snapshots, snapshotInfo{Code: code, At: snap.At, SHA256: hash(body)})
	}
	return snapshots, data.Save(filepath.Join(dst, "funds.json"))
}

func render(view func(*ui.Context), w, h int, dark, vibrant bool) *image.RGBA {
	t := ui.NewTesterWith(view, w, h, ui.TesterOptions{Dark: dark, Preferences: ui.Preferences{ReduceMotion: true}})
	t.SetScale(scale)
	if vibrant {
		t.SetVibrancy(true)
	}
	t.Frame()
	return t.Image()
}

// Remove only transparent space beneath the content, keeping every UI pixel.
func trim(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				return img.SubImage(image.Rect(0, 0, b.Max.X, y+1)).(*image.RGBA)
			}
		}
	}
	return img
}

func panelMaterial(raw *image.RGBA, dark bool) *image.RGBA {
	w, h := raw.Bounds().Dx()/scale, raw.Bounds().Dy()/scale
	bitmap := ui.NewBitmap(raw)
	return render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			from, to := ui.Hex("#dadee9"), ui.Hex("#f4f4f8")
			if dark {
				from, to = ui.Hex("#3c3c44"), ui.Hex("#74747c")
			}
			p.Clip(r, 12, func() {
				p.FillGradient(r, ui.LinearGradient{From: from, To: to, Angle: 180}, 12)
				p.Image(bitmap, r, ui.Contain)
			})
		})
	}, w, h, dark, false)
}

func poster(main, panel *image.RGBA, icon ui.ImageSource, dark bool) *image.RGBA {
	mainBitmap, panelBitmap := ui.NewBitmap(main), ui.NewBitmap(panel)
	ink, muted, accent := ui.Hex("#182641"), ui.Hex("#61718a"), ui.Hex("#4e5cd6")
	if dark {
		ink, muted, accent = ui.Hex("#f0f2fc"), ui.Hex("#9caac5"), ui.Hex("#a4b1ff")
	}
	return render(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			background(p, r, dark)
			p.Shadow(ui.Rect{X: 48, Y: 40, W: 54, H: 54}, 13, 0, 5, 14, 0, ui.Hex("#4b49a4").Alpha(.16))
			p.Image(icon, ui.Rect{X: 48, Y: 40, W: 54, H: 54}, ui.Contain)
			window(p, mainBitmap, ui.Rect{X: 48, Y: 232, W: 1080, H: 840}, dark)
			window(p, panelBitmap, ui.Rect{X: 1190, Y: 470, W: 320, H: float32(panel.Bounds().Dy()) / scale}, dark)
			p.Line(1190, 433, 1230, 433, 2, accent.Alpha(.55))
		})
		label := func(x, y, size float32, weight int, color ui.Color, s string) {
			ui.Text(c, s).Absolute().Left(x).Top(y).FontSize(size).FontWeight(weight).TextColor(color).NoWrap()
		}
		label(118, 49, 30, 650, ink, "FundBar")
		label(48, 112, 52, 700, ink, "基金动态，抬眼可见。")
		label(50, 183, 19, 400, muted, "盘中估值  ·  自选分组  ·  净值走势  ·  持仓收益")
		label(1234, 52, 13, 500, accent, "为你的 Mac 而作")
		label(1234, 76, 13, 400, muted, "原生应用  /  开源  /  无需注册")
		label(1190, 349, 26, 650, ink, "随时看一眼。")
		label(1190, 389, 15, 400, muted, "点击菜单栏，展开你的基金。")
		bottom := 470 + float32(panel.Bounds().Dy())/scale
		label(1190, bottom+35, 17, 550, ink, "持有与关注，分组查看")
		label(1190, bottom+64, 14, 400, muted, "打开主面板，继续了解每一笔细节。")
		label(50, 1092, 12, 400, muted, "当前界面渲染 · 示例持仓 · 缓存行情仅作展示，盘中估值不等于实际净值")
		label(1333, 1092, 12, 500, muted, "FundBar for macOS")
	}, posterW, posterH, dark, false)
}

func window(p *ui.Painter, bitmap *ui.Bitmap, r ui.Rect, dark bool) {
	shadow, edge := ui.Hex("#344770").Alpha(.20), ui.Hex("#ffffff").Alpha(.85)
	if dark {
		shadow, edge = ui.Hex("#000000").Alpha(.50), ui.Hex("#8e9ccc").Alpha(.30)
	}
	p.Shadow(r, 13, 0, 18, 44, 0, shadow)
	p.Shadow(r, 13, 0, 2, 6, 0, shadow.Alpha(.13))
	p.Clip(r, 13, func() { p.Image(bitmap, r, ui.Contain) })
	p.Stroke(r, edge, 13, 1)
}

func background(p *ui.Painter, r ui.Rect, dark bool) {
	from, to := ui.Hex("#f8faff"), ui.Hex("#e9eef9")
	glow, rim := ui.Hex("#bab3ef"), ui.Hex("#ffffff").Alpha(.72)
	glass := ui.Hex("#acbadd").Alpha(.12)
	if dark {
		from, to = ui.Hex("#121929"), ui.Hex("#202c46")
		glow, rim, glass = ui.Hex("#5b51ac"), ui.Hex("#abbbea").Alpha(.14), ui.Hex("#8998d3").Alpha(.06)
	}
	p.FillGradient(r, ui.LinearGradient{From: from, To: to, Angle: 145}, 0)
	p.FillRadialGradient(r, ui.RadialGradient{Center: ui.Point{X: .89, Y: .65}, Radius: 720,
		Stops: []ui.GradientStop{{Color: glow.Alpha(.40), At: 0}, {Color: glow.Alpha(0), At: 1}}}, 0)
	// Wide translucent arcs stay behind the windows, never across the UI.
	for i := 0; i < 3; i++ {
		x := float32(i) * 70
		var ribbon ui.Path
		ribbon.MoveTo(1640+x, 460).CubeTo(1310+x, 540, 1120+x, 850, 1190+x, 1160).
			LineTo(1300+x, 1160).CubeTo(1190+x, 870, 1410+x, 570, 1650+x, 535).Close()
		p.FillPathGradient(&ribbon, ui.LinearGradient{From: glass, To: rim.Alpha(.05), Angle: 150})
		var rimPath ui.Path
		rimPath.MoveTo(1640+x, 460).CubeTo(1310+x, 540, 1120+x, 850, 1190+x, 1160)
		p.StrokePath(&rimPath, 1.5, rim)
	}
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func hash(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
