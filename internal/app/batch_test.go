package app

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBatchCodes(t *testing.T) {
	cases := []struct {
		text  string
		codes []string
		ok    bool
	}{
		{"161725 005827", []string{"161725", "005827"}, true},
		{"161725,005827", []string{"161725", "005827"}, true},
		{"161725，005827", []string{"161725", "005827"}, true},
		{"161725、005827；012414", []string{"161725", "005827", "012414"}, true},
		{"161725\n005827", []string{"161725", "005827"}, true},
		{"161725, 005827、012414；161725\n012414",
			[]string{"161725", "005827", "012414"}, true},
		// A duplicate is still two codes typed, and is added once.
		{"161725 161725", []string{"161725"}, true},
		// A single code searches for it as before.
		{"161725", nil, false},
		// A token that is not six digits is ordinary search text.
		{"16172 005827", nil, false},
		{"1617252 005827", nil, false},
		{"白酒 161725", nil, false},
		{"161725 白酒 005827", nil, false},
		{"", nil, false},
		{"  ", nil, false},
	}
	for _, c := range cases {
		codes, ok := batchCodes(c.text)
		if ok != c.ok || !reflect.DeepEqual(codes, c.codes) {
			t.Errorf("batchCodes(%q) = %v, %v; want %v, %v",
				c.text, codes, ok, c.codes, c.ok)
		}
	}
}

func TestBatchRowAddsTheCodesOnce(t *testing.T) {
	a := preview(t)
	// 161725 is already followed; 012414 is new, and repeated.
	a.query = "161725, 012414, 012414"
	tt := ui.NewTester(a.View, 1080, 780)
	tt.Frame()
	for _, text := range []string{"添加这 1 只基金", "161725  已添加", "012414"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
	if err := tt.Click("添加这 1 只基金"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.query != "" || a.hits != nil {
		t.Fatalf("the batch left query = %q, hits = %v", a.query, a.hits)
	}
	if got := codes(a.all); len(got) != 3 || got[2] != "012414" {
		t.Fatalf("funds = %v, want 012414 added once", got)
	}
	if f := a.byCode("012414"); f == nil || !f.In(holdingGroup) {
		t.Fatal("012414 did not join the group")
	}

	// Typing the same codes again marks both as already there and counts
	// none.
	a.query = "161725, 012414"
	tt.Frame()
	if !tt.HasText("添加这 0 只基金") || !tt.HasText("012414  已添加") {
		t.Fatalf("the row after adding = %v", tt.Texts())
	}
	if err := tt.Click("添加这 0 只基金"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := codes(a.all); len(got) != 3 {
		t.Fatalf("clicking again added more: %v", got)
	}
}

func TestBatchRowJoinsTheShownGroup(t *testing.T) {
	a := preview(t)
	id := watchGroup(t, a, "关注")
	a.query = "161725 012414"
	tt := ui.NewTester(a.View, 1080, 780)
	tt.Frame()
	if err := tt.Click("添加这 2 只基金"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, code := range []string{"161725", "012414"} {
		f := a.byCode(code)
		if f == nil || !f.In(id) {
			t.Fatalf("%s did not join the shown group", code)
		}
	}
	// 161725 was already in the holding group, and stays there; 012414 is
	// only in the made group, as Add would leave it.
	if !a.byCode("161725").In(holdingGroup) {
		t.Fatal("161725 left the holding group")
	}
	if a.byCode("012414").In(holdingGroup) {
		t.Fatal("012414 joined the holding group too")
	}
}
