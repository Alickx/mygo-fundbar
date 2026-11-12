package app

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
)

// shareTester draws the picture's view in a window tall enough for every
// row, without rendering the PNG.
func shareTester(a *App, money bool) *ui.Tester {
	tt := ui.NewTester(func(c *ui.Context) { a.shareView(c, money) }, 720, 1200)
	tt.Frame()
	return tt
}

func TestShareViewNamesTheFundsAndTheirPercents(t *testing.T) {
	a := preview(t)
	tt := shareTester(a, false)
	for _, f := range a.inGroup(holdingGroup) {
		if !tt.HasText(fundName(f)) {
			t.Fatalf("the picture does not name %q: %v", fundName(f), tt.Texts())
		}
		if !tt.HasText(pct(f.ChangePct())) {
			t.Fatalf("the picture does not show %s for %q: %v",
				pct(f.ChangePct()), fundName(f), tt.Texts())
		}
	}
	if !tt.HasText("基金估值") || !tt.HasText("已公布净值") {
		t.Fatalf("the picture is missing its heading: %v", tt.Texts())
	}
	if !tt.HasText("估值仅供参考，以基金公司公布为准 · FundBar") {
		t.Fatalf("the picture is missing its foot: %v", tt.Texts())
	}
}

// exactText reports that a text of the frame is exactly s, as HasText, which
// looks for a part, cannot say of a date the heading already holds.
func exactText(tt *ui.Tester, s string) bool {
	for _, text := range tt.Texts() {
		if text == s {
			return true
		}
	}
	return false
}

// When the funds' numbers are of different days, every row says its own day,
// so that the dates read as a column rather than as a forgotten few. When
// they are all of one day, only the heading names it.
func TestShareRowsDateThemselvesOnlyWhenTheDaysDiffer(t *testing.T) {
	a := preview(t)
	for _, f := range a.funds {
		f.HasEst, f.QuoteAt = false, time.Time{}
	}
	a.funds[0].Navs = []model.Nav{{Date: "2026-11-11", Unit: 0.5859, Pct: -0.63}}
	a.funds[1].Navs = []model.Nav{{Date: "2026-11-10", Unit: 1.3993, Pct: -0.21}}

	tt := shareTester(a, false)
	if !tt.HasText("2026-11-11") {
		t.Fatalf("the heading does not name the newest day: %v", tt.Texts())
	}
	for _, label := range []string{"11-11", "11-10"} {
		if !exactText(tt, label) {
			t.Fatalf("the rows do not form a date column, %q missing: %v",
				label, tt.Texts())
		}
	}

	// One day for the lot: no row repeats the heading's date.
	a.funds[1].Navs = []model.Nav{{Date: "2026-11-11", Unit: 1.3993, Pct: -0.21}}
	tt = shareTester(a, false)
	if exactText(tt, "11-11") {
		t.Fatalf("a row on the heading's day repeats it: %v", tt.Texts())
	}
}

// The window offers the share picture where the user is looking, and not
// when there is nothing to draw.
func TestTheWindowOffersTheShareMenu(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 780)
	tt.Frame()
	if !tt.HasText("分享") {
		t.Fatalf("the window offers no share menu: %v", tt.Texts())
	}

	var got []byte
	a.writeImage = func(data []byte) error {
		got = append([]byte(nil), data...)
		return nil
	}
	if err := tt.Click("分享"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("复制图片（仅涨跌幅）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(got) == 0 {
		t.Fatal("the window's share menu copied nothing")
	}
}

func TestSharePictureLeavesMoneyOutUnlessAsked(t *testing.T) {
	a := preview(t)
	plain := shareTester(a, false)
	for _, unwanted := range []string{"元", "市值", "份额", "今日收益"} {
		if plain.HasText(unwanted) {
			t.Fatalf("the default picture contains %q: %v", unwanted, plain.Texts())
		}
	}
	// The change in percent is still there: it is what the picture is for.
	if !plain.HasText(pct(a.Summary().Pct)) {
		t.Fatalf("the default picture lost the day's change: %v", plain.Texts())
	}

	money := shareTester(a, true)
	for _, wanted := range []string{"今日收益", "元"} {
		if !money.HasText(wanted) {
			t.Fatalf("the money picture is missing %q: %v", wanted, money.Texts())
		}
	}
}

func TestSharePictureNeverShowsAWatchedFundsAmount(t *testing.T) {
	a := preview(t)
	// The second fund is only watched: it has no shares, so no amount of
	// its own may appear even in the money picture.
	a.all[1].Shares = 0
	tt := shareTester(a, true)
	amounts := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, "元") {
			amounts++
		}
	}
	// One line for the group's gain and one for the fund that holds shares;
	// the watched fund gets none.
	want := 1
	for _, f := range a.inGroup(holdingGroup) {
		if f.Shares > 0 {
			want++
		}
	}
	if amounts != want {
		t.Fatalf("amount lines = %d, want %d: %v", amounts, want, tt.Texts())
	}
}

func TestSharePictureIsWideAtTwoX(t *testing.T) {
	a := preview(t)
	img := a.shareImage(false)
	if got := img.Bounds().Dx(); got != shareWidth*shareScale {
		t.Fatalf("width = %d, want %d", got, shareWidth*shareScale)
	}
	if got := img.Bounds().Dy(); got <= 0 || got >= 1600*shareScale {
		t.Fatalf("height = %d, want the content's own", got)
	}
}

func TestCopyShareSendsPNGThroughTheField(t *testing.T) {
	a := preview(t)
	var got []byte
	a.writeImage = func(data []byte) error {
		got = append([]byte(nil), data...)
		return nil
	}
	a.CopyShare(false)
	if len(got) == 0 {
		t.Fatal("nothing reached the clipboard field")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("the clipboard bytes are not a PNG: %v", err)
	}
	if cfg.Width != shareWidth*shareScale {
		t.Fatalf("copied picture width = %d, want %d", cfg.Width, shareWidth*shareScale)
	}
	if !strings.Contains(a.flashNow(), "已复制分享图，可直接粘贴") {
		t.Fatalf("status = %q", a.flashNow())
	}
}

func TestSaveShareWritesThePicture(t *testing.T) {
	a := preview(t)
	// The date comes from the numbers, not from the clock: the picture is of
	// the day shown, even when it is opened after midnight.
	for _, f := range a.all {
		f.HasEst, f.QuoteAt = false, time.Time{}
		f.Navs = []model.Nav{{Date: "2026-11-11", Unit: 1}}
	}
	var opts mygo.SaveDialogOptions
	path := filepath.Join(t.TempDir(), "poster.png")
	a.saveFile = func(o mygo.SaveDialogOptions) (string, error) {
		opts = o
		return path, nil
	}
	a.SaveShare()
	if !strings.HasSuffix(opts.DefaultPath, "FundBar 今日收益 2026-11-11.png") {
		t.Fatalf("default name = %q", opts.DefaultPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the saved file is not a PNG: %v", err)
	}
	if cfg.Width != shareWidth*shareScale {
		t.Fatalf("saved width = %d", cfg.Width)
	}
	if !strings.Contains(a.flashNow(), "已保存分享图") {
		t.Fatalf("status = %q", a.flashNow())
	}
}

func TestPortfolioShareButtonCopiesThePicture(t *testing.T) {
	a := preview(t)
	a.PreviewPortfolio(true)
	var got []byte
	a.writeImage = func(data []byte) error {
		got = append([]byte(nil), data...)
		return nil
	}
	tt := ui.NewTester(a.View, 1080, 1000)
	tt.Frame()
	if err := tt.Click("分享"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(got) == 0 {
		t.Fatal("the 分享 button copied nothing")
	}
	if !strings.Contains(a.flashNow(), "已复制分享图，可直接粘贴") {
		t.Fatalf("status = %q", a.flashNow())
	}
}

func TestShareMenuIsDisabledWithoutHoldings(t *testing.T) {
	empty := New(t.TempDir())
	tt := ui.NewTester(empty.View, 1080, 780)
	tt.Frame()
	if tt.HasText("分享") {
		t.Fatalf("a window with nothing to share offers one: %v", tt.Texts())
	}
	for _, it := range empty.TrayMenu().Items() {
		if it.Label != "分享今日收益" {
			continue
		}
		if !it.Disabled {
			t.Fatal("the share submenu is enabled without holding funds")
		}
		for _, sub := range it.Submenu {
			if !sub.Disabled {
				t.Fatalf("%q is enabled without holding funds", sub.Label)
			}
		}
		return
	}
	t.Fatal("the tray menu has no share submenu")
}

func TestShareMenuWorksWithHoldings(t *testing.T) {
	a := preview(t)
	for _, it := range a.TrayMenu().Items() {
		if it.Label != "分享今日收益" {
			continue
		}
		if it.Disabled {
			t.Fatal("the share submenu is disabled with holding funds")
		}
		var labels []string
		for _, sub := range it.Submenu {
			labels = append(labels, sub.Label)
			if sub.Disabled {
				t.Fatalf("%q is disabled with holding funds", sub.Label)
			}
		}
		want := []string{"复制图片（仅涨跌幅）", "复制图片（含金额）", "保存图片…"}
		if strings.Join(labels, "|") != strings.Join(want, "|") {
			t.Fatalf("share items = %v, want %v", labels, want)
		}
		return
	}
	t.Fatal("the tray menu has no share submenu")
}

func TestSharePictureCountsTheFundsBeyondTen(t *testing.T) {
	a := preview(t)
	for i := 0; i < 10; i++ {
		a.all = append(a.all, &Fund{
			Code: fmt.Sprintf("9%05d", i), Name: fmt.Sprintf("额外基金%d", i),
			Groups: []string{holdingGroup}, Loaded: true,
			Navs: []model.Nav{{Date: now().Format("2006-01-02"), Unit: 1}},
		})
	}
	tt := shareTester(a, false)
	if !tt.HasText("另有 2 只") {
		t.Fatalf("the picture does not count the rest: %v", tt.Texts())
	}
}
