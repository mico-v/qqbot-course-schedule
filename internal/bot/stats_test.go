package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/render"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/timing"
)

func TestRenderFooterTextAppendsPlaceholder(t *testing.T) {
	got := renderFooterText("实时状态 · 课程时间以本地时区为准")
	want := "实时状态 · 课程时间以本地时区为准 · " + render.FooterTimingPlaceholder
	if got != want {
		t.Fatalf("renderFooterText = %q, want %q", got, want)
	}
	if got := renderFooterText("   "); got != render.FooterTimingPlaceholder {
		t.Fatalf("blank footer = %q, want the bare placeholder", got)
	}
}

func TestReplierRenderFooter(t *testing.T) {
	var r *Replier
	if got := r.RenderFooter(); got != "" {
		t.Fatalf("nil replier footer = %q, want empty", got)
	}
	r = NewReplier(&Inbound{}, nil)
	if got := r.RenderFooter(); got != "" {
		t.Fatalf("unrendered footer = %q, want empty", got)
	}
	r.stats.RenderMS = 132
	if got := r.RenderFooter(); got != "渲染用时 132ms" {
		t.Fatalf("footer = %q", got)
	}
	if got := r.WithRenderFooter("实时状态"); got != "实时状态 · 渲染用时 132ms" {
		t.Fatalf("WithRenderFooter = %q", got)
	}
	if got := r.WithRenderFooter(""); got != "渲染用时 132ms" {
		t.Fatalf("empty footer = %q", got)
	}
}

func TestClaimStatsStageKeepsLatest(t *testing.T) {
	r := NewReplier(&Inbound{}, nil)
	if !r.claimStatsStage(schedule.StatsStageHandler) {
		t.Fatal("the first stage must be accepted")
	}
	if r.claimStatsStage(schedule.StatsStageHandler) {
		t.Fatal("a repeated stage must be refused, so one message is one row")
	}
	if !r.claimStatsStage(schedule.StatsStageReply) {
		t.Fatal("a later stage must supersede an earlier one")
	}
	if !r.claimStatsStage(schedule.StatsStageCard) {
		t.Fatal("the card stage must supersede a reply")
	}
	if r.claimStatsStage(schedule.StatsStageReply) {
		t.Fatal("a card record must not be replaced by the reply it fell back to")
	}
}

func TestInboundReceivedAtFallsBackToNow(t *testing.T) {
	if at := (&Inbound{}).ReceivedAt(); at.IsZero() {
		t.Fatal("an unstamped inbound must report a usable time")
	}
	want := time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	in := NewInbound(timing.WithReceived(context.Background(), want), OriginGroup, "G", "U", "m", "", "", "", "", nil, nil)
	if got := in.ReceivedAt(); !got.Equal(want) {
		t.Fatalf("ReceivedAt = %v, want %v", got, want)
	}
}

// TestDayCardPipelineRecordsRenderStats drives a real /今日课表 and asserts the
// card footer carries the timing and exactly one record lands in storage.
func TestDayCardPipelineRecordsRenderStats(t *testing.T) {
	env := newStatsTestEnv(t)
	handler := NewDefaultHandler(env)
	in := &Inbound{Origin: OriginGroup, GroupOpenID: "GROUP", UserOpenID: "U1", MsgID: "m1"}
	in.Content = "/今日课表"
	received := time.Now()
	ctx := timing.WithReceived(context.Background(), received)

	r := env.newReplier(ctx, in)
	handler.Dispatch(ctx, in, r)

	if r.Stats().RenderMS <= 0 {
		t.Fatalf("render stage was not measured: %+v", r.Stats())
	}
	records := loadStats(t, env)
	if len(records) != 1 {
		t.Fatalf("records = %d, want exactly 1 (a card must not also record its fallback reply): %+v", len(records), records)
	}
	got := records[0]
	if got.Stage != schedule.StatsStageCard {
		t.Fatalf("stage = %q, want %q", got.Stage, schedule.StatsStageCard)
	}
	if got.Command != "/今日课表" {
		t.Fatalf("command = %q, want /今日课表", got.Command)
	}
	if got.ScopeID != schedule.ScopeGroup("GROUP") {
		t.Fatalf("scope = %q", got.ScopeID)
	}
	if got.RenderMS <= 0 {
		t.Fatalf("render_ms = %d, want > 0", got.RenderMS)
	}
	if !got.OK {
		t.Fatal("a successful card must record ok")
	}
	if !got.ReceivedAt.Equal(received) {
		t.Fatalf("received_at = %v, want the stamped arrival %v", got.ReceivedAt, received)
	}
}

// TestTextReplyRecordsStats covers the non-card path.
func TestTextReplyRecordsStats(t *testing.T) {
	env := newStatsTestEnv(t)
	handler := NewDefaultHandler(env)
	in := &Inbound{Origin: OriginGroup, GroupOpenID: "GROUP", UserOpenID: "U1", MsgID: "m1"}
	in.Content = "/ping"
	ctx := timing.WithReceived(context.Background(), time.Now())

	handler.Dispatch(ctx, in, env.newReplier(ctx, in))

	records := loadStats(t, env)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1: %+v", len(records), records)
	}
	if records[0].Stage != schedule.StatsStageReply {
		t.Fatalf("stage = %q, want %q", records[0].Stage, schedule.StatsStageReply)
	}
	// A reply against a local fake server finishes inside one millisecond, and
	// Milliseconds truncates, so assert on the shape rather than a magnitude.
	if records[0].ServerMS < 0 || records[0].SendMS < 0 {
		t.Fatalf("durations must not be negative: %+v", records[0])
	}
	if records[0].RenderMS != 0 {
		t.Fatalf("render_ms = %d, want 0 on a text reply", records[0].RenderMS)
	}
	if !records[0].OK {
		t.Fatal("a successful reply must record ok")
	}
}

// TestUnmatchedCommandStillRecords covers the paths that send nothing, which
// would otherwise leave no trace in the table.
func TestUnmatchedCommandStillRecords(t *testing.T) {
	env := newStatsTestEnv(t)
	handler := NewDefaultHandler(env)
	in := &Inbound{Origin: OriginGroup, GroupOpenID: "GROUP", UserOpenID: "U1", MsgID: "m1"}
	in.Content = "你好啊"
	ctx := timing.WithReceived(context.Background(), time.Now())

	handler.Dispatch(ctx, in, env.newReplier(ctx, in))

	records := loadStats(t, env)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1: %+v", len(records), records)
	}
	if records[0].Stage != schedule.StatsStageHandler {
		t.Fatalf("stage = %q, want %q", records[0].Stage, schedule.StatsStageHandler)
	}
	if records[0].Command != "" {
		t.Fatalf("command = %q, want empty for an unmatched message", records[0].Command)
	}
}

func TestInboundCommandTaggedOnMatch(t *testing.T) {
	env := newStatsTestEnv(t)
	handler := NewDefaultHandler(env)
	in := &Inbound{Origin: OriginGroup, GroupOpenID: "GROUP", UserOpenID: "U1", MsgID: "m1"}
	in.Content = "课表"
	handler.Dispatch(context.Background(), in, env.newReplier(context.Background(), in))
	if in.Command != "/课表" {
		t.Fatalf("command = %q, want /课表 (the platform panel drops the slash)", in.Command)
	}
}

// loadStats reads every record currently stored.
func loadStats(t *testing.T, env *Env) []schedule.MessageStats {
	t.Helper()
	records, err := env.Store.ListMessageStats(time.Now().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("ListMessageStats: %v", err)
	}
	return records
}

// newStatsTestEnv builds an Env wired to a fake QQ API and a real store, with a
// scope that already has a schedule so the day card renders.
func newStatsTestEnv(t *testing.T) *Env {
	t.Helper()
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)

	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	ctx := context.Background()
	freshImport := freshMessage(base)
	if err := env.ImportICS(ctx, freshImport, env.newReplier(ctx, freshImport), Attachment{
		URL: icsServer.URL + "/schedule.ics", Filename: "schedule.ics",
	}); err != nil {
		t.Fatalf("ImportICS: %v", err)
	}
	// The import replies, so it records one row; drop it so each test starts
	// from an empty table.
	if records := loadStats(t, env); len(records) != 1 {
		t.Fatalf("seeding records = %d, want 1 (the import replies)", len(records))
	}
	if _, err := env.Store.PruneMessageStats(time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("PruneMessageStats: %v", err)
	}
	if records := loadStats(t, env); len(records) != 0 {
		t.Fatalf("table not cleared, %d rows left", len(records))
	}
	return env
}

// TestStatsLogModeReading checks the environment switch, which is not otherwise
// exercised because the default keeps quiet.
func TestStatsLogModeReading(t *testing.T) {
	cases := map[string]statsLogMode{
		"":      statsLogSlow,
		"slow":  statsLogSlow,
		"off":   statsLogOff,
		"none":  statsLogOff,
		"0":     statsLogOff,
		"all":   statsLogAll,
		"debug": statsLogAll,
	}
	for value, want := range cases {
		t.Setenv(statsLogEnvVar, value)
		if got := currentStatsLogMode(); got != want {
			t.Errorf("%s=%q: mode = %v, want %v", statsLogEnvVar, value, got, want)
		}
	}
}

// TestRecordStatsNeverPanicsWithoutService guards the nil-receiver paths that
// proactive pushes and tests rely on.
func TestRecordStatsNeverPanicsWithoutService(t *testing.T) {
	var env *Env
	env.recordStats(context.Background(), &Inbound{}, NewReplier(&Inbound{}, nil), schedule.StatsStageCard, nil)

	empty := &Env{}
	empty.recordStats(context.Background(), &Inbound{}, NewReplier(&Inbound{}, nil), schedule.StatsStageCard, nil)

	if got := strings.TrimSpace(renderFooterText("")); got == "" {
		t.Fatal("empty footer must still yield the placeholder")
	}
}

// TestDispatchInstallsRecorder guards a bug where a caller that builds a bare
// replier (as the webhook dispatcher does) lost the reply and card stages, so
// every real message recorded only the handler row.
func TestDispatchInstallsRecorder(t *testing.T) {
	env := newStatsTestEnv(t)
	handler := NewDefaultHandler(env)
	in := &Inbound{Origin: OriginGroup, GroupOpenID: "GROUP", UserOpenID: "U1", MsgID: "m1"}
	in.Content = "/ping"
	ctx := timing.WithReceived(context.Background(), time.Now())

	// Deliberately bypass env.newReplier, the way webhook.Dispatcher does.
	handler.Dispatch(ctx, in, NewReplier(in, env.Client))

	records := loadStats(t, env)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1: %+v", len(records), records)
	}
	if records[0].Stage != schedule.StatsStageReply {
		t.Fatalf("stage = %q, want %q: Dispatch must install the recorder it was not given",
			records[0].Stage, schedule.StatsStageReply)
	}
}
