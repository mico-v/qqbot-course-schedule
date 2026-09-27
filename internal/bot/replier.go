package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
	"github.com/mico-v/qqbot-course-schedule/internal/schedule"
	"github.com/mico-v/qqbot-course-schedule/internal/timing"
)

// maxPassiveReplies is the platform limit for replies to one msg_id.
const maxPassiveReplies = 5

// ErrPassiveLimit means the 5-reply window for the message is exhausted.
var ErrPassiveLimit = errors.New("被动回复次数已达上限(5)")

// Replier sends replies and proactive messages for one inbound message. It owns
// the client and the passive-reply counter, keeping both out of Inbound.
type Replier struct {
	Inbound *Inbound
	Client  *qqapi.Client

	seqMu sync.Mutex
	seq   int

	// statsMu guards recorded, which is the latest stage written for this
	// message. Later stages replace earlier ones rather than adding rows.
	statsMu  sync.Mutex
	recorded schedule.StatsStage

	// renderDone is when the card image finished rendering, so the send stage
	// can report render-completion → send-return separately from the whole.
	// Zero when the message involved no rendered card.
	renderDone time.Time

	// stats accumulates per-stage timings for the handling record.
	stats MessageStats

	// statsCtx is set by Env so a finished send can be recorded without the
	// replier holding a back-reference to Env.
	statsCtx *statsCtx
}

// MessageStats is how long each stage of handling one message took. Zero means
// the stage did not run.
type MessageStats struct {
	// RenderMS is from message receipt to a rendered card image on disk.
	RenderMS int
	// UploadMS is from the rendered card to the message call returning, which
	// covers the media upload and the send request together.
	UploadMS int
	// SendMS is the duration of the final send call.
	SendMS int
	// ReplyMS is from message receipt to a text/file reply returning. It is
	// only set on the stages that do not push a card.
	ReplyMS int
}

// NewReplier returns a replier for one inbound message.
func NewReplier(in *Inbound, client *qqapi.Client) *Replier {
	return &Replier{Inbound: in, Client: client}
}

// NextSeq consumes one passive reply slot.
func (r *Replier) NextSeq() (int, error) {
	r.seqMu.Lock()
	defer r.seqMu.Unlock()
	if r.seq >= maxPassiveReplies {
		return 0, ErrPassiveLimit
	}
	r.seq++
	return r.seq, nil
}

// Stats returns the accumulated stage timings.
func (r *Replier) Stats() MessageStats {
	if r == nil {
		return MessageStats{}
	}
	return r.stats
}

// markRender records the render stage duration and when the image was ready.
func (r *Replier) markRender(start time.Time) {
	if r != nil {
		r.stats.RenderMS = int(time.Since(start).Milliseconds())
		r.renderDone = time.Now()
	}
}

// markSend records the send stage duration and, for a card path, the span from
// render completion to the send returning. renderDone is the zero time when the
// message did not involve a rendered card.
func (r *Replier) markSend(start, renderDone time.Time) {
	if r == nil {
		return
	}
	r.stats.SendMS = int(time.Since(start).Milliseconds())
	if !renderDone.IsZero() {
		r.stats.UploadMS = int(time.Since(renderDone).Milliseconds())
	}
}

// RenderFooter is the timing suffix appended to a card footer.
func (r *Replier) RenderFooter() string {
	if r == nil || r.stats.RenderMS <= 0 {
		return ""
	}
	return fmt.Sprintf("渲染用时 %dms", r.stats.RenderMS)
}

// WithRenderFooter appends the render timing to a card footer text.
func (r *Replier) WithRenderFooter(footer string) string {
	suffix := r.RenderFooter()
	if suffix == "" {
		return footer
	}
	if strings.TrimSpace(footer) == "" {
		return suffix
	}
	return footer + " · " + suffix
}

// recordStats persists one completed handling record and logs it. It never
// blocks or fails a reply: a storage or logging problem is reported and
// dropped, because the user's answer already went out.
//
// Only the latest stage of one message is kept, so a card send does not also
// record the reply it fell back to.
func (e *Env) recordStats(ctx context.Context, in *Inbound, r *Replier, stage schedule.StatsStage, err error) {
	if e == nil || e.Service == nil || in == nil || r == nil {
		return
	}
	if !r.claimStatsStage(stage) {
		return
	}
	received, ok := timing.Received(ctx)
	if !ok {
		// Messages built without a stamp (proactive pushes) have no origin to
		// measure from; the per stage durations are still worth keeping.
		received = in.ReceivedAt()
	}
	stats := r.Stats()
	record := schedule.MessageStats{
		ScopeID:    e.Scope(in),
		Origin:     string(in.Origin),
		Command:    in.Command,
		UserID:     in.UserOpenID,
		Stage:      stage,
		ReceivedAt: received,
		RenderMS:   stats.RenderMS,
		UploadMS:   stats.UploadMS,
		SendMS:     stats.SendMS,
		OK:         err == nil,
	}
	if stage != schedule.StatsStageHandler {
		record.ServerMS = int(time.Since(received).Milliseconds())
	}
	if code := qqapi.ErrorCode(err); code != 0 {
		record.ErrCode = strconv.Itoa(code)
	}
	if saveErr := e.Service.RecordMessageStats(record); saveErr != nil {
		slog.Warn("记录处理耗时失败", "err", saveErr)
	}
	e.logStats(record)
}

// logStats reports one record. The default verbosity only surfaces slow
// handling, so a busy group does not fill the journal with routine timings.
func (e *Env) logStats(record schedule.MessageStats) {
	switch currentStatsLogMode() {
	case statsLogOff:
		return
	case statsLogSlow:
		if record.TotalMS() < int(schedule.StatsSlowThreshold.Milliseconds()) {
			return
		}
	}
	slog.Info("处理耗时",
		"origin", record.Origin,
		"scope", shortID(record.ScopeID),
		"cmd", record.Command,
		"stage", string(record.Stage),
		"render_ms", record.RenderMS,
		"send_ms", record.SendMS,
		"server_ms", record.ServerMS,
		"ok", record.OK,
	)
}

// Reply sends a passive text reply and consumes one of the five reply slots.
func (r *Replier) Reply(ctx context.Context, text string) error {
	seq, err := r.NextSeq()
	if err != nil {
		return err
	}
	in := r.Inbound
	start := time.Now()
	switch in.Origin {
	case OriginGroup:
		if in.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		if in.MsgID == "" && in.EventID != "" {
			err = r.Client.SendGroupTextEvent(ctx, in.GroupOpenID, text, in.EventID)
			break
		}
		_, err = r.Client.SendGroupText(ctx, in.GroupOpenID, text, in.MsgID, seq)
	case OriginPrivate:
		if in.UserOpenID == "" {
			return errors.New("单聊消息缺少 user_openid")
		}
		if in.MsgID == "" && in.EventID != "" {
			err = r.Client.SendC2CTextEvent(ctx, in.UserOpenID, text, in.EventID)
			break
		}
		_, err = r.Client.SendC2CText(ctx, in.UserOpenID, text, in.MsgID, seq)
	default:
		return fmt.Errorf("未知消息来源 %q", in.Origin)
	}
	r.markSend(start, time.Time{})
	r.afterSend(ctx, schedule.StatsStageReply, err)
	return err
}

// ReplyImage sends a passive image message from a public URL.
func (r *Replier) ReplyImage(ctx context.Context, imageURL string) error {
	seq, err := r.NextSeq()
	if err != nil {
		return err
	}
	in := r.Inbound
	start := time.Now()
	switch in.Origin {
	case OriginGroup:
		if in.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		if in.MsgID == "" && in.EventID != "" {
			err = r.Client.SendGroupImageEvent(ctx, in.GroupOpenID, imageURL, in.EventID)
			break
		}
		err = r.Client.SendGroupImage(ctx, in.GroupOpenID, imageURL, in.MsgID, seq)
	case OriginPrivate:
		if in.UserOpenID == "" {
			return errors.New("单聊消息缺少 user_openid")
		}
		if in.MsgID == "" && in.EventID != "" {
			err = r.Client.SendC2CImageEvent(ctx, in.UserOpenID, imageURL, in.EventID)
			break
		}
		err = r.Client.SendC2CImage(ctx, in.UserOpenID, imageURL, in.MsgID, seq)
	default:
		return fmt.Errorf("未知消息来源 %q", in.Origin)
	}
	r.markSend(start, time.Time{})
	return err
}

// ReplyFile sends a passive file message from a public URL.
func (r *Replier) ReplyFile(ctx context.Context, fileURL, fileName string) error {
	seq, err := r.NextSeq()
	if err != nil {
		return err
	}
	in := r.Inbound
	start := time.Now()
	switch in.Origin {
	case OriginGroup:
		if in.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		err = r.Client.SendGroupFile(ctx, in.GroupOpenID, fileURL, fileName, in.MsgID, seq)
	case OriginPrivate:
		if in.UserOpenID == "" {
			return errors.New("单聊消息缺少 user_openid")
		}
		err = r.Client.SendC2CFile(ctx, in.UserOpenID, fileURL, fileName, in.MsgID, seq)
	default:
		return fmt.Errorf("未知消息来源 %q", in.Origin)
	}
	r.markSend(start, time.Time{})
	r.afterSend(ctx, schedule.StatsStageReply, err)
	return err
}

// PushImage sends a proactive image message with no msg_id.
func (r *Replier) PushImage(ctx context.Context, imageURL string) error {
	in := r.Inbound
	start := time.Now()
	var err error
	switch in.Origin {
	case OriginGroup:
		err = r.Client.SendGroupImage(ctx, in.GroupOpenID, imageURL, "", 0)
	case OriginPrivate:
		err = r.Client.SendC2CImage(ctx, in.UserOpenID, imageURL, "", 0)
	default:
		return fmt.Errorf("未知消息来源 %q", in.Origin)
	}
	r.markSend(start, time.Time{})
	return err
}

// Push sends a proactive text message with no msg_id.
func (r *Replier) Push(ctx context.Context, text string) error {
	in := r.Inbound
	start := time.Now()
	var err error
	switch in.Origin {
	case OriginGroup:
		_, err = r.Client.SendGroupText(ctx, in.GroupOpenID, text, "", 0)
	case OriginPrivate:
		_, err = r.Client.SendC2CText(ctx, in.UserOpenID, text, "", 0)
	default:
		return fmt.Errorf("未知消息来源 %q", in.Origin)
	}
	r.markSend(start, time.Time{})
	return err
}

// statsLogMode is how much of the timing record reaches the log.
type statsLogMode int

const (
	statsLogSlow statsLogMode = iota
	statsLogAll
	statsLogOff
)

// statsLogEnvVar selects what the timing log reports. The default keeps the
// journal quiet by printing only handling slower than schedule.StatsSlowThreshold.
const statsLogEnvVar = "QQBOT_STATS_LOG"

func currentStatsLogMode() statsLogMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(statsLogEnvVar))) {
	case "off", "none", "0":
		return statsLogOff
	case "all", "1", "debug":
		return statsLogAll
	default:
		return statsLogSlow
	}
}

// claimStatsStage reports whether this stage supersedes what is already
// recorded for the message. A later stage always wins, so the final send
// replaces an earlier handler-level record instead of adding a second row.
func (r *Replier) claimStatsStage(stage schedule.StatsStage) bool {
	if r == nil {
		return false
	}
	r.statsMu.Lock()
	defer r.statsMu.Unlock()
	if statsStageRank(stage) <= statsStageRank(r.recorded) {
		return false
	}
	r.recorded = stage
	return true
}

func statsStageRank(stage schedule.StatsStage) int {
	switch stage {
	case schedule.StatsStageHandler:
		return 1
	case schedule.StatsStageReply:
		return 2
	case schedule.StatsStageCard:
		return 3
	default:
		return 0
	}
}

// recorder persists one completed handling record. Env installs it when it
// builds a replier, which keeps the replier free of a back-reference to Env.
type recorder func(ctx context.Context, in *Inbound, r *Replier, stage schedule.StatsStage, err error)

// statsCtx carries the message context a record is attributed to. It is the
// dispatch context, not the command's, so the arrival stamp survives.
type statsCtx struct {
	ctx    context.Context
	record recorder
}

// WithRecorder installs the callback that persists completed handling records
// and the context those records are attributed to. Returning the receiver keeps
// construction to one expression.
func (r *Replier) WithRecorder(ctx context.Context, record recorder) *Replier {
	if r != nil {
		r.statsCtx = &statsCtx{ctx: ctx, record: record}
	}
	return r
}

// afterSend reports a finished send stage to the recorder, when one is set.
func (r *Replier) afterSend(ctx context.Context, stage schedule.StatsStage, err error) {
	if r == nil || r.statsCtx == nil || r.statsCtx.record == nil {
		return
	}
	recordCtx := ctx
	if r.statsCtx.ctx != nil {
		recordCtx = r.statsCtx.ctx
	}
	r.statsCtx.record(recordCtx, r.Inbound, r, stage, err)
}
