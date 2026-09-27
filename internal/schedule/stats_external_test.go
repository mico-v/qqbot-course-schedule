package schedule_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/store"
)

func newStatsService(t *testing.T) (*schedule.Service, *store.Store) {
	t.Helper()
	handle, err := store.Open(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { handle.Close() })
	return schedule.NewService(handle), handle
}

func at(minutesAgo int) time.Time {
	return time.Now().Add(-time.Duration(minutesAgo) * time.Minute)
}

func TestRecordMessageStatsRequiresScope(t *testing.T) {
	service, _ := newStatsService(t)
	if err := service.RecordMessageStats(schedule.MessageStats{}); err == nil {
		t.Fatal("a record without scope_id must be rejected")
	}
	if err := service.RecordMessageStats(schedule.MessageStats{ScopeID: "  "}); err == nil {
		t.Fatal("a whitespace scope_id must be rejected")
	}
}

func TestRecordMessageStatsClampsNegativeDurations(t *testing.T) {
	service, handle := newStatsService(t)
	err := service.RecordMessageStats(schedule.MessageStats{
		ScopeID:  "group:G1",
		Stage:    schedule.StatsStageCard,
		RenderMS: -5,
		SendMS:   -1,
		UploadMS: -100,
		ServerMS: -7,
	})
	if err != nil {
		t.Fatalf("RecordMessageStats: %v", err)
	}
	records, err := handle.ListMessageStats(at(1), "")
	if err != nil {
		t.Fatalf("ListMessageStats: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	got := records[0]
	if got.RenderMS != 0 || got.SendMS != 0 || got.UploadMS != 0 || got.ServerMS != 0 {
		t.Fatalf("negative durations not clamped: %+v", got)
	}
}

func TestRecordMessageStatsFillsMissingTimestamp(t *testing.T) {
	service, handle := newStatsService(t)
	if err := service.RecordMessageStats(schedule.MessageStats{ScopeID: "group:G1"}); err != nil {
		t.Fatalf("RecordMessageStats: %v", err)
	}
	records, _ := handle.ListMessageStats(at(1), "")
	if len(records) != 1 || records[0].ReceivedAt.IsZero() {
		t.Fatalf("received_at was not filled: %+v", records)
	}
}

func TestMessageStatsRoundTrip(t *testing.T) {
	service, _ := newStatsService(t)
	received := at(3).Truncate(time.Millisecond)
	want := schedule.MessageStats{
		ScopeID:    "group:G1",
		Origin:     "group",
		Command:    "/今日课表",
		UserID:     "U1",
		Stage:      schedule.StatsStageCard,
		ReceivedAt: received,
		RenderMS:   137,
		UploadMS:   88,
		SendMS:     95,
		ServerMS:   320,
		OK:         true,
	}
	if err := service.RecordMessageStats(want); err != nil {
		t.Fatalf("RecordMessageStats: %v", err)
	}
	got, err := service.MessageStatsSummary(at(10), "group:G1")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if got.Count != 1 {
		t.Fatalf("count = %d, want 1", got.Count)
	}
	// TotalMS is render + send.
	if got.AverageMS != want.TotalMS() {
		t.Fatalf("average = %d, want %d", got.AverageMS, want.TotalMS())
	}
	if got.RenderAverageMS != want.RenderMS {
		t.Fatalf("render average = %d, want %d", got.RenderAverageMS, want.RenderMS)
	}
	if got.Failed != 0 {
		t.Fatalf("failed = %d, want 0", got.Failed)
	}
}

func TestMessageStatsSummaryByCommand(t *testing.T) {
	service, _ := newStatsService(t)
	for _, record := range []schedule.MessageStats{
		{ScopeID: "group:G1", Command: "/今日课表", ReceivedAt: at(5), RenderMS: 100, SendMS: 100, OK: true},
		{ScopeID: "group:G1", Command: "/今日课表", ReceivedAt: at(4), RenderMS: 200, SendMS: 200, OK: true},
		{ScopeID: "group:G1", Command: "/课表", ReceivedAt: at(3), RenderMS: 50, SendMS: 50, OK: true},
		{ScopeID: "group:G1", Command: "", ReceivedAt: at(2), RenderMS: 10, SendMS: 10, OK: false},
	} {
		if err := service.RecordMessageStats(record); err != nil {
			t.Fatalf("RecordMessageStats: %v", err)
		}
	}
	summary, err := service.MessageStatsSummary(at(10), "group:G1")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if summary.Count != 4 {
		t.Fatalf("count = %d, want 4", summary.Count)
	}
	if summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", summary.Failed)
	}
	dayCard, ok := summary.ByCommand["/今日课表"]
	if !ok {
		t.Fatalf("ByCommand missing /今日课表: %+v", summary.ByCommand)
	}
	if dayCard.Count != 2 || dayCard.AverageMS != 300 || dayCard.MaxMS != 400 {
		t.Fatalf("/今日课表 summary = %+v", dayCard)
	}
	if _, ok := summary.ByCommand["（无指令）"]; !ok {
		t.Fatalf("commandless records must group under （无指令）: %+v", summary.ByCommand)
	}
}

func TestMessageStatsSummaryScopesAndWindow(t *testing.T) {
	service, _ := newStatsService(t)
	records := []schedule.MessageStats{
		{ScopeID: "group:G1", ReceivedAt: at(5), OK: true},
		{ScopeID: "group:G2", ReceivedAt: at(5), OK: true},
		{ScopeID: "group:G1", ReceivedAt: at(120), OK: true},
	}
	for _, record := range records {
		if err := service.RecordMessageStats(record); err != nil {
			t.Fatalf("RecordMessageStats: %v", err)
		}
	}
	scoped, err := service.MessageStatsSummary(at(10), "group:G1")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if scoped.Count != 1 {
		t.Fatalf("scoped count = %d, want 1 (window and scope must both apply)", scoped.Count)
	}
	all, err := service.MessageStatsSummary(at(10), "")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if all.Count != 2 {
		t.Fatalf("all-scope count = %d, want 2", all.Count)
	}
}

func TestMessageStatsSummaryEmpty(t *testing.T) {
	service, _ := newStatsService(t)
	summary, err := service.MessageStatsSummary(at(10), "group:G1")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if summary.Count != 0 || summary.AverageMS != 0 || summary.MaxMS != 0 {
		t.Fatalf("empty summary must be zeroed: %+v", summary)
	}
}

func TestPruneMessageStatsDropsOnlyExpired(t *testing.T) {
	service, _ := newStatsService(t)
	now := time.Now()
	old := schedule.MessageStats{ScopeID: "group:G1", ReceivedAt: now.Add(-schedule.StatsRetention - time.Hour), OK: true}
	fresh := schedule.MessageStats{ScopeID: "group:G1", ReceivedAt: now.Add(-time.Minute), OK: true}
	for _, record := range []schedule.MessageStats{old, fresh} {
		if err := service.RecordMessageStats(record); err != nil {
			t.Fatalf("RecordMessageStats: %v", err)
		}
	}
	removed, err := service.PruneMessageStats(now)
	if err != nil {
		t.Fatalf("PruneMessageStats: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	summary, err := service.MessageStatsSummary(now.Add(-24*time.Hour), "")
	if err != nil {
		t.Fatalf("MessageStatsSummary: %v", err)
	}
	if summary.Count != 1 {
		t.Fatalf("remaining = %d, want 1", summary.Count)
	}
}

func TestMessageStatsTotalMS(t *testing.T) {
	record := schedule.MessageStats{RenderMS: 100, UploadMS: 50, SendMS: 25, ServerMS: 200}
	if got := record.TotalMS(); got != 125 {
		t.Fatalf("TotalMS = %d, want 125 (render + send)", got)
	}
}

func TestStatsStageValues(t *testing.T) {
	// Stored as text, so the literals are part of the schema.
	cases := map[schedule.StatsStage]string{
		schedule.StatsStageHandler: "handler",
		schedule.StatsStageCard:    "card",
		schedule.StatsStageReply:   "reply",
	}
	for stage, want := range cases {
		if string(stage) != want {
			t.Errorf("stage %v = %q, want %q", stage, string(stage), want)
		}
	}
}

func TestRecordMessageStatsPropagatesStoreFailure(t *testing.T) {
	service := schedule.NewService(failingStorage{})
	err := service.RecordMessageStats(schedule.MessageStats{ScopeID: "group:G1"})
	if !errors.Is(err, errInsertRefused) {
		t.Fatalf("err = %v, want errInsertRefused", err)
	}
}

var errInsertRefused = errors.New("insert refused")

// failingStorage refuses stats writes while satisfying the rest of the port.
type failingStorage struct{}

func (failingStorage) InsertMessageStats(schedule.MessageStats) error { return errInsertRefused }
func (failingStorage) ListMessageStats(time.Time, string) ([]schedule.MessageStats, error) {
	return nil, nil
}
func (failingStorage) PruneMessageStats(time.Time) (int, error) { return 0, nil }

func (failingStorage) GetMember(string, string) (*schedule.Member, bool, error) {
	return nil, false, nil
}
func (failingStorage) GetScopeMembers(string) (map[string]*schedule.Member, error) { return nil, nil }
func (failingStorage) PutMember(string, string, *schedule.Member, *int64) error    { return nil }
func (failingStorage) ListDayOverrides(string) ([]schedule.DayOverrideRow, error)  { return nil, nil }
func (failingStorage) DeleteScopeDayOverrides(string) error                        { return nil }
func (failingStorage) ListScopeSummaries() ([]schedule.ScopeSummary, error)        { return nil, nil }
func (failingStorage) SetDayOverride(string, string, string, schedule.DayOverride, string, string) error {
	return nil
}
func (failingStorage) DeleteDayOverride(string, string, string) (bool, error) { return false, nil }
func (failingStorage) GetKV(string, string, string, any) (bool, error)        { return false, nil }
func (failingStorage) ListKV(string, string) ([]schedule.KVEntry, error)      { return nil, nil }
func (failingStorage) SetKV(string, string, string, any) error                { return nil }
func (failingStorage) DeleteKV(string, string, string) error                  { return nil }
