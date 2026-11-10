package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"fundbar/internal/model"
	"fundbar/internal/source"
	"fundbar/internal/store"
)

// sourceHit is a fund the search found.
func sourceHit(code, name string) source.Hit {
	return source.Hit{Code: code, Name: name}
}

// watchGroup makes a group the user named and shows it.
func watchGroup(t *testing.T, a *App, name string) string {
	t.Helper()
	a.beginNewGroup()
	a.groupDraft = name
	a.CommitGroup()
	if a.groupEditing {
		t.Fatalf("the group %q was not made: %s", name, a.groupErr)
	}
	if a.group == "" {
		t.Fatalf("the group %q was not shown", name)
	}
	return a.group
}

func TestGroupListsOnlyItsFunds(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	if len(a.funds) != 0 {
		t.Fatalf("a new group lists %d funds", len(a.funds))
	}
	// Following a fund of the holding group again lists it here too, as the
	// search of a group does.
	a.Add(sourceHit("161725", "招商中证白酒指数(LOF)A"))
	if len(a.funds) != 1 || a.funds[0].Code != "161725" {
		t.Fatalf("the group lists %v", codes(a.funds))
	}
	if !a.byCode("161725").In(id) {
		t.Fatal("the fund did not join the group")
	}
	a.SelectGroup(holdingGroup)
	if len(a.funds) != 2 {
		t.Fatalf("the holding group lists %v", codes(a.funds))
	}
}

func TestNewFundOfAGroupStaysOutOfTheHoldingGroup(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	a.all = append(a.all, &Fund{
		Code: "000001", Name: "只看不买", Groups: []string{id}, Loaded: true,
		Navs: []model.Nav{{Date: now().Format("2006-01-02"), Unit: 1, Pct: 2}},
	})
	a.rebuildList()
	if len(a.funds) != 1 {
		t.Fatalf("the group lists %v", codes(a.funds))
	}
	if held := codes(a.inGroup(holdingGroup)); len(held) != 2 || held[0] != "161725" || held[1] != "005827" {
		t.Fatalf("the holding group lists %v", held)
	}
}

func TestMenuBarAddsUpTheHoldingGroupAlone(t *testing.T) {
	a := preview(t)
	before := a.Summary()
	id := watchGroup(t, a, "关注")
	a.all = append(a.all, &Fund{
		Code: "000001", Name: "只看不买", Groups: []string{id}, Loaded: true,
		Navs: []model.Nav{{Date: now().Format("2006-01-02"), Unit: 1, Pct: 9}},
	})
	a.rebuildList()
	after := a.Summary()
	if after != before {
		t.Fatalf("the menu bar changed from %+v to %+v", before, after)
	}
	// The group says how its funds moved, and nothing of what is held.
	group := a.GroupSummary(id)
	if group.Holding || group.Up != 1 || group.Pct != 9 {
		t.Fatalf("group summary = %+v", group)
	}
}

func TestMadeGroupShowsNoHolding(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	f := a.all[0]
	f.Groups = append(f.Groups, id)
	a.rebuildList()
	if len(a.funds) != 1 {
		t.Fatalf("the group lists %v", codes(a.funds))
	}

	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	for _, text := range []string{"分组平均涨跌", "关注"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
	for _, text := range []string{"我的持仓", "录入持仓", "累计收益"} {
		if tt.HasText(text) {
			t.Fatalf("a made group shows %q", text)
		}
	}
	// The row says the fund is followed, not held.
	if !tt.HasText("关注") {
		t.Fatal("expected the row to say the fund is only watched")
	}
}

func TestPanelSwitchesGroups(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	a.Add(sourceHit("161725", "招商中证白酒指数(LOF)A"))
	tt := panelTester(a)
	tt.Frame()
	if !tt.HasText("关注") {
		t.Fatalf("missing the group in %v", tt.Texts())
	}
	if err := tt.Click("持有"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.group != holdingGroup {
		t.Fatalf("group = %q, want the holding group", a.group)
	}
	if len(a.funds) != 2 {
		t.Fatalf("the holding group lists %v", codes(a.funds))
	}
	if err := tt.Click("关注"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.group != id {
		t.Fatalf("group = %q, want %q", a.group, id)
	}
}

func TestGroupIsMadeFromTheBar(t *testing.T) {
	a := preview(t)
	tt := ui.NewTester(a.View, 1080, 900)
	tt.Frame()
	if err := tt.Click("＋"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.groupEditing {
		t.Fatal("expected the field of a new group")
	}
	tt.Type("关注")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.groups) != 1 || a.groups[0].Name != "关注" {
		t.Fatalf("groups = %+v", a.groups)
	}
	if a.group != a.groups[0].ID {
		t.Fatal("expected the new group to be shown")
	}
	if !tt.HasText("关注") || !tt.HasText("持有") {
		t.Fatalf("the bar shows %v", tt.Texts())
	}
}

func TestGroupsAreSavedAndRead(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	a.Add(sourceHit("161725", "招商中证白酒指数(LOF)A"))
	a.all[1].Groups = append(a.all[1].Groups, id)
	a.save()

	b := New(a.dir)
	b.Load()
	if b.group != id || len(b.groups) != 1 || b.groups[0].Name != "关注" {
		t.Fatalf("groups = %+v, group = %q", b.groups, b.group)
	}
	if got := codes(b.funds); len(got) != 2 || got[0] != "161725" || got[1] != "005827" {
		t.Fatalf("the group lists %v", got)
	}
	if !b.byCode("005827").In(holdingGroup) {
		t.Fatal("the holding group lost a fund")
	}
	// Each group keeps the fund it showed.
	b.SelectGroup(holdingGroup)
	if got := b.funds[b.sel].Code; got != "161725" {
		t.Fatalf("the holding group shows %q", got)
	}
	b.SelectGroup(id)
	if got := b.funds[b.sel].Code; got != "161725" {
		t.Fatalf("the made group shows %q", got)
	}
}

func TestLeavingAndDeletingGroups(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	a.Add(sourceHit("161725", "招商中证白酒指数(LOF)A"))
	// It is in both groups: leaving the made one keeps it in the holding
	// group.
	a.LeaveGroup(a.sel)
	if a.byCode("161725").In(id) {
		t.Fatal("the fund is still in the group")
	}
	if !a.byCode("161725").In(holdingGroup) {
		t.Fatal("the fund left the holding group too")
	}
	// A fund only in the group goes away with it.
	a.all = append(a.all, &Fund{Code: "000001", Name: "只看不买", Groups: []string{id}})
	a.rebuildList()
	a.DeleteGroup(id)
	if a.byCode("000001") != nil {
		t.Fatal("a fund only in the deleted group is still followed")
	}
	if a.byCode("161725") == nil || a.group != holdingGroup {
		t.Fatal("deleting a group took the holding group with it")
	}
}

func TestReorderKeepsTheOtherGroupsOrder(t *testing.T) {
	a := preview(t)
	a.all = append(a.all, &Fund{Code: "028678", Name: "红利", Groups: []string{holdingGroup}})
	// 005827 stays only held; the other two join a group.
	id := watchGroup(t, a, "关注")
	a.all[0].Groups = append(a.all[0].Groups, id)
	a.all[2].Groups = append(a.all[2].Groups, id)
	a.rebuildList()
	if got := codes(a.funds); len(got) != 2 || got[0] != "161725" || got[1] != "028678" {
		t.Fatalf("the group lists %v", got)
	}
	// Move the last row of the group above the first.
	a.reorder([]int{1}, 0)
	a.SelectGroup(holdingGroup)
	if got := codes(a.funds); got[0] != "028678" || got[1] != "161725" || got[2] != "005827" {
		t.Fatalf("the holding group lists %v", got)
	}
	a.SelectGroup(id)
	if got := codes(a.funds); got[0] != "028678" || got[1] != "161725" {
		t.Fatalf("the group lists %v", got)
	}
}

func TestOlderFilesLandInTheHoldingGroup(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"funds":[{"code":"161725","shares":12000,"cost":0.62}],"selected":"161725"}`
	if err := os.WriteFile(filepath.Join(dir, "funds.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(dir)
	a.Load()
	if a.group != holdingGroup || len(a.groups) != 0 {
		t.Fatalf("group = %q, groups = %+v", a.group, a.groups)
	}
	if got := codes(a.funds); len(got) != 1 || got[0] != "161725" {
		t.Fatalf("the holding group lists %v", got)
	}
	if f := a.all[0]; !f.In(holdingGroup) || f.Shares != 12000 {
		t.Fatalf("fund = %+v", f)
	}
}

func codes(funds []*Fund) []string {
	out := make([]string, len(funds))
	for i, f := range funds {
		out[i] = f.Code
	}
	return out
}

// manyGroups gives the app n groups named by their place, and shows the
// last.
func manyGroups(a *App, n int) string {
	for i := 1; i <= n; i++ {
		a.groups = append(a.groups, store.Group{
			ID: fmt.Sprintf("g%d", i), Name: fmt.Sprintf("分组%d", i),
		})
	}
	id := fmt.Sprintf("g%d", n)
	a.SelectGroup(id)
	return id
}

func TestManyGroupsKeepTheActiveOneAndAMoreMenu(t *testing.T) {
	a := preview(t)
	id := manyGroups(a, 12)
	tt := panelTester(a)
	tt.Frame()
	if !tt.HasText("⋯") {
		t.Fatalf("missing the more menu in %v", tt.Texts())
	}
	if !tt.HasText("分组12") {
		t.Fatalf("the active group is not a chip in %v", tt.Texts())
	}
	if tt.HasText("分组5") {
		t.Fatal("a group of the more menu shows as a chip")
	}

	if err := tt.Click("⋯"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	found, renamed := false, false
	for _, label := range menu {
		if label == "分组5" {
			found = true
		}
		if label == "重命名「分组12」" {
			renamed = true
		}
	}
	if !found || !renamed {
		t.Fatalf("menu = %v", menu)
	}
	if err := tt.ChooseMenuItem("分组5"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.group != "g5" {
		t.Fatalf("group = %q, want g5", a.group)
	}
	if !tt.HasText("分组5") {
		t.Fatalf("the chosen group is not a chip in %v", tt.Texts())
	}
	if a.group == id {
		t.Fatal("the group did not change")
	}
}

func TestMoreMenuRenamesTheActiveGroup(t *testing.T) {
	a := preview(t)
	manyGroups(a, 12)
	tt := panelTester(a)
	tt.Frame()
	if err := tt.Click("⋯"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名「分组12」"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.groupEditing || a.groupDraft != "分组12" {
		t.Fatalf("editing = %v, draft = %q", a.groupEditing, a.groupDraft)
	}
}

func TestGroupFitShowsEveryGroupOnce(t *testing.T) {
	a := preview(t)
	manyGroups(a, 12)
	var shown, rest []string
	tt := ui.NewTester(func(c *ui.Context) {
		t := theme(c)
		c.SetTheme(t)
		shown, rest = a.groupFit(c, 296)
	}, 320, 60)
	tt.Frame()
	if !groupShown(shown, a.group) {
		t.Fatalf("the active group is not shown: %v", shown)
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, shown...), rest...) {
		if seen[id] {
			t.Fatalf("%q is in the bar twice", id)
		}
		seen[id] = true
	}
	if len(seen) != len(a.groups)+1 {
		t.Fatalf("the bar accounts for %d of %d groups", len(seen), len(a.groups)+1)
	}
}
