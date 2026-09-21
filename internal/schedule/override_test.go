package schedule

import (
	"strings"
	"testing"
	"time"
)

func day(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", value, LocalTZ)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestResolveOverrideTargets(t *testing.T) {
	members := map[string]*Member{
		"A": {UserID: "A", Name: "小明"},
		"B": {UserID: "B", Name: "小红"},
	}
	cases := []struct {
		name      string
		person    string
		sender    string
		isGroup   bool
		isAdmin   bool
		mentions  []Mention
		want      []string
		errSubstr string
	}{
		{name: "admin defaults to everyone", person: "", sender: "A", isGroup: true, isAdmin: true, want: []string{DayOverrideAll}},
		{name: "member defaults to self", person: "", sender: "A", isGroup: true, want: []string{"A"}},
		{name: "private defaults to self", person: "", sender: "A", isGroup: false, want: []string{"A"}},
		{name: "own words", person: "我", sender: "A", isGroup: true, want: []string{"A"}},
		{name: "everyone as admin", person: "全体", sender: "A", isGroup: true, isAdmin: true, want: []string{DayOverrideAll}},
		{name: "everyone as member", person: "全体", sender: "A", isGroup: true, errSubstr: "只有管理员"},
		{name: "own nickname", person: "小明", sender: "A", isGroup: true, want: []string{"A"}},
		{name: "other nickname as member", person: "小红", sender: "A", isGroup: true, errSubstr: "普通成员只能标记自己"},
		{name: "other nickname as admin", person: "小红", sender: "A", isGroup: true, isAdmin: true, want: []string{"B"}},
		{name: "openid as member", person: "B", sender: "A", isGroup: true, errSubstr: "普通成员只能标记自己"},
		{name: "openid as admin", person: "B", sender: "A", isGroup: true, isAdmin: true, want: []string{"B"}},
		{name: "unknown", person: "小刚", sender: "A", isGroup: true, isAdmin: true, errSubstr: "没有找到成员"},
		{name: "mention fallback", person: "小刚", sender: "A", isGroup: true, isAdmin: true, mentions: []Mention{{ID: "C", Name: "小刚"}}, want: []string{"C"}},
		{name: "private cannot target", person: "小明", sender: "A", isGroup: false, errSubstr: "私聊只能标记自己"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			targets, errMsg := ResolveOverrideTargets(members, testCase.sender, testCase.isGroup, testCase.isAdmin, testCase.person, testCase.mentions)
			if testCase.errSubstr != "" {
				if errMsg == "" || !strings.Contains(errMsg, testCase.errSubstr) {
					t.Fatalf("errMsg = %q, want contains %q", errMsg, testCase.errSubstr)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("unexpected error: %q", errMsg)
			}
			if strings.Join(targets, ",") != strings.Join(testCase.want, ",") {
				t.Fatalf("targets = %v, want %v", targets, testCase.want)
			}
		})
	}
}

func TestSetAndClearDayOverrides(t *testing.T) {
	storage := newMemoryStorage()
	service := NewService(storage)
	members := map[string]*Member{
		"A": {UserID: "A", Name: "小明"},
		"B": {UserID: "B", Name: "小红"},
	}
	today := day(t, "2026-09-17")
	scope := "group:G1"

	text, err := service.SetDayOverrides(scope, []string{DayOverrideAll}, []time.Time{day(t, "2026-10-01")}, DayOverrideHoliday, nil, "A", members, today)
	if err != nil {
		t.Fatalf("SetDayOverrides: %v", err)
	}
	if !strings.Contains(text, "已将 2026-10-01 标记为休假（全体成员）") {
		t.Fatalf("text = %q", text)
	}
	rows, _ := storage.ListDayOverrides(scope)
	if len(rows) != 1 || rows[0].Kind != DayOverrideHoliday || rows[0].UserID != DayOverrideAll {
		t.Fatalf("rows = %+v", rows)
	}

	// Shift over an existing holiday on the same day reports the replacement.
	if _, err := service.SetDayOverrides(scope, []string{DayOverrideAll}, []time.Time{day(t, "2026-10-11")}, DayOverrideHoliday, nil, "A", members, today); err != nil {
		t.Fatal(err)
	}
	source := day(t, "2026-10-08")
	text, err = service.SetDayOverrides(scope, []string{DayOverrideAll}, []time.Time{day(t, "2026-10-11")}, DayOverrideShift, &source, "A", members, today)
	if err != nil {
		t.Fatalf("shift: %v", err)
	}
	if !strings.Contains(text, "标记为调休") || !strings.Contains(text, "原有标记已被覆盖") {
		t.Fatalf("shift text = %q", text)
	}
	rows, _ = storage.ListDayOverrides(scope)
	for _, row := range rows {
		if row.Day == "2026-10-11" && (row.Kind != DayOverrideShift || row.SourceDay != "2026-10-08") {
			t.Fatalf("shift row = %+v", row)
		}
	}

	// Past dates get a note.
	text, err = service.SetDayOverrides(scope, []string{"A"}, []time.Time{day(t, "2026-09-01")}, DayOverrideHoliday, nil, "A", members, today)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "包含已过去的日期") {
		t.Fatalf("past text = %q", text)
	}

	// A member cannot remove a scope-wide marker.
	text, err = service.ClearDayOverrides(scope, []string{"A"}, []time.Time{day(t, "2026-10-01")}, members)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "面向全体成员的标记") {
		t.Fatalf("scope-wide clear text = %q", text)
	}

	// The admin removes it.
	text, err = service.ClearDayOverrides(scope, []string{DayOverrideAll}, []time.Time{day(t, "2026-10-01")}, members)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "已取消 2026-10-01 的休假标记（全体成员）") {
		t.Fatalf("clear text = %q", text)
	}

	// /假期 lists what is left.
	list, err := service.DayOverrideListText(scope, members, today)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "调休：按 2026-10-08 的课程上课") {
		t.Fatalf("list = %q", list)
	}
}

func TestSetDayOverridesValidation(t *testing.T) {
	service := NewService(newMemoryStorage())
	members := map[string]*Member{"A": {UserID: "A", Name: "小明"}}
	today := day(t, "2026-09-17")
	scope := "group:G1"

	if _, err := service.SetDayOverrides(scope, []string{"A"}, nil, DayOverrideHoliday, nil, "A", members, today); err == nil {
		t.Fatal("empty days should fail")
	}
	if _, err := service.SetDayOverrides(scope, []string{"A"}, []time.Time{day(t, "2026-10-11")}, DayOverrideShift, nil, "A", members, today); err == nil {
		t.Fatal("shift without source should fail")
	}
	same := day(t, "2026-10-11")
	if _, err := service.SetDayOverrides(scope, []string{"A"}, []time.Time{same}, DayOverrideShift, &same, "A", members, today); err == nil {
		t.Fatal("shift with same source should fail")
	}
	far := day(t, "2028-10-11")
	if _, err := service.SetDayOverrides(scope, []string{"A"}, []time.Time{day(t, "2026-10-11")}, DayOverrideShift, &far, "A", members, today); err == nil {
		t.Fatal("shift beyond max span should fail")
	}
}
