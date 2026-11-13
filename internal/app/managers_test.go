package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/source"
)

func TestManagerChangeIsAChangeSinceTheCurrentTeam(t *testing.T) {
	f := &Fund{ManagerTerms: []source.ManagerTerm{
		{Start: "2025-04-12", End: "至今", Names: []string{"甲"}},
		{Start: "2021-01-01", End: "2025-04-11", Names: []string{"乙", "丙"}},
	}}
	at := time.Date(2026, 11, 13, 10, 53, 0, 0, cst)
	since, names, ok := f.managerChange(at)
	if !ok || since.Format("2006-01-02") != "2025-04-12" {
		t.Fatalf("managerChange = %v, %v, %v", since, names, ok)
	}
	if len(names) != 1 || names[0] != "甲" {
		t.Fatalf("names = %v", names)
	}
	// A fund that has had one team since its first day has not changed
	// managers, however new it is.
	young := &Fund{ManagerTerms: []source.ManagerTerm{
		{Start: "2025-01-01", End: "至今", Names: []string{"甲"}},
	}}
	if _, _, ok := young.managerChange(at); ok {
		t.Fatal("a first team is not a change")
	}
	// The offer follows the change, not the current team's birthday: four
	// years old is too long ago.
	f.ManagerTerms[0].Start = "2022-11-12"
	if _, _, ok := f.managerChange(at); ok {
		t.Fatal("a four-year-old change still offered its window")
	}
	f.ManagerTerms[0].Start = "2023-11-13"
	if _, _, ok := f.managerChange(at); !ok {
		t.Fatal("a change exactly three years old was left out")
	}
}

func TestManagerWindowStartsAtTheChange(t *testing.T) {
	f := &Fund{Navs: []model.Nav{
		{Date: "2025-04-10", Unit: 1.00},
		{Date: "2025-04-14", Unit: 1.01},
		{Date: "2025-04-15", Unit: 1.02},
		{Date: "2025-04-16", Unit: 1.03},
	}}
	since := day("2025-04-12")
	navs, from, partial, label := chartWindow(f, 0, since)
	if label != "现任经理以来" || partial {
		t.Fatalf("window = %q, partial %v", label, partial)
	}
	if from.Format("2006-01-02") != "2025-04-12" {
		t.Fatalf("window opens at %v", from)
	}
	if len(navs) != 3 || navs[0].Date != "2025-04-14" {
		t.Fatalf("window = %+v", navs)
	}
	// The manager window does not fall back to the fund's own start, unlike a
	// period longer than the fund's history.
	if label == "成立以来" {
		t.Fatal("the manager window was renamed to the fund's history")
	}
}

func TestManagerFilterIsOfferedOnlyAfterAChange(t *testing.T) {
	a := preview(t)
	f := a.funds[0]
	last := now()
	f.Navs = nil
	for i := 120; i >= 0; i-- {
		f.Navs = append(f.Navs, model.Nav{
			Date: last.AddDate(0, 0, -i).Format("2006-01-02"),
			Unit: 1 + float64(120-i)*0.001,
		})
	}
	f.ManagerTerms = []source.ManagerTerm{
		{Start: last.AddDate(0, 0, -100).Format("2006-01-02"), End: "至今", Names: []string{"甲"}},
		{Start: last.AddDate(0, 0, -400).Format("2006-01-02"),
			End: last.AddDate(0, 0, -101).Format("2006-01-02"), Names: []string{"乙"}},
	}
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	if err := tt.Click("⋯"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("现任经理管理以来"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.managerOnly {
		t.Fatal("the chart did not choose the manager window")
	}
	if !tt.HasText("经理以来") || !tt.HasText("现任经理以来净值变化") {
		t.Fatalf("the manager window is not named: %v", tt.Texts())
	}

	// A fund whose only team has been there since its first day has no
	// change to offer, and the saved filter falls back to its period.
	f.ManagerTerms = f.ManagerTerms[:1]
	tt.Frame()
	if tt.HasText("现任经理以来净值变化") {
		t.Fatal("the manager window stayed on a fund without a change")
	}
	if err := tt.Click("⋯"); err != nil {
		t.Fatal(err)
	}
	for _, item := range tt.Menu() {
		if item == "现任经理管理以来" {
			t.Fatalf("menu offered the manager window: %v", tt.Menu())
		}
	}
	tt.Frame()

	// Choosing a period takes the chart back to it.
	f.ManagerTerms = []source.ManagerTerm{
		{Start: last.AddDate(0, 0, -100).Format("2006-01-02"), End: "至今", Names: []string{"甲"}},
		{Start: last.AddDate(0, 0, -400).Format("2006-01-02"),
			End: last.AddDate(0, 0, -101).Format("2006-01-02"), Names: []string{"乙"}},
	}
	tt.Frame()
	if err := tt.Click("1月"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.managerOnly {
		t.Fatal("a period did not leave the manager window")
	}
	if !tt.HasText("近1月净值变化") {
		t.Fatalf("the period did not come back: %v", tt.Texts())
	}
}
