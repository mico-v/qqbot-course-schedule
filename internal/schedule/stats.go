package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// StatsStage names the point that finished a handling record. Only the latest
// stage of one message is kept, so a card send does not also record the reply
// it fell back to.
type StatsStage string

const (
	// StatsStageHandler is the whole dispatch: receipt to the handler returning.
	StatsStageHandler StatsStage = "handler"
	// StatsStageCard is a card message: receipt to the send returning.
	StatsStageCard StatsStage = "card"
	// StatsStageReply is a text or file reply: receipt to the send returning.
	StatsStageReply StatsStage = "reply"
)

// StatsRetention is how long handling records are kept.
const StatsRetention = 7 * 24 * time.Hour

// StatsSlowThreshold is the duration above which a record is logged at the
// default log verbosity.
const StatsSlowThreshold = 2 * time.Second

// MessageStats is one completed message handling record.
type MessageStats struct {
	ScopeID    string     `json:"scope_id"`
	Origin     string     `json:"origin"`
	Command    string     `json:"command"`
	UserID     string     `json:"user_id"`
	Stage      StatsStage `json:"stage"`
	ReceivedAt time.Time  `json:"received_at"`
	// RenderMS is receipt to a rendered card image on disk.
	RenderMS int `json:"render_ms"`
	// UploadMS is render completion to the send returning, covering the media
	// upload and the send request together.
	UploadMS int `json:"upload_ms"`
	// SendMS is the duration of the final send call alone.
	SendMS int `json:"send_ms"`
	// ServerMS is receipt to the send returning. Zero on the handler stage,
	// which has nothing to send.
	ServerMS int    `json:"server_ms"`
	OK       bool   `json:"ok"`
	ErrCode  string `json:"err_code"`
}

// TotalMS is the sum of the measured stages, useful for ordering records.
func (m MessageStats) TotalMS() int {
	return m.RenderMS + m.SendMS
}

// StatsSummary is an aggregate over a set of records.
type StatsSummary struct {
	Count int `json:"count"`
	// AverageMS, MedianMS, P90MS, P99MS and MaxMS describe the total duration.
	AverageMS int `json:"average_ms"`
	MedianMS  int `json:"median_ms"`
	P90MS     int `json:"p90_ms"`
	P99MS     int `json:"p99_ms"`
	MaxMS     int `json:"max_ms"`
	// RenderAverageMS is the average render stage over records that rendered.
	RenderAverageMS int `json:"render_average_ms"`
	Failed          int `json:"failed"`
	// ByCommand breaks the totals down per command prefix.
	ByCommand map[string]StatsSummary `json:"by_command,omitempty"`
}

// summarizeStats reduces records to a summary. A non-nil byCommand request
// fills ByCommand recursively (without nesting further).
func summarizeStats(records []MessageStats, byCommand bool) StatsSummary {
	summary := StatsSummary{Count: len(records), ByCommand: nil}
	if len(records) == 0 {
		return summary
	}
	totals := make([]int, 0, len(records))
	renderSum, renderCount := 0, 0
	for _, record := range records {
		total := record.TotalMS()
		totals = append(totals, total)
		if record.RenderMS > 0 {
			renderSum += record.RenderMS
			renderCount++
		}
		if !record.OK {
			summary.Failed++
		}
	}
	sort.Ints(totals)
	sum := 0
	for _, value := range totals {
		sum += value
	}
	summary.AverageMS = sum / len(totals)
	summary.MedianMS = percentile(totals, 0.50)
	summary.P90MS = percentile(totals, 0.90)
	summary.P99MS = percentile(totals, 0.99)
	summary.MaxMS = totals[len(totals)-1]
	if renderCount > 0 {
		summary.RenderAverageMS = renderSum / renderCount
	}
	if !byCommand {
		return summary
	}
	groups := make(map[string][]MessageStats)
	for _, record := range records {
		key := record.Command
		if key == "" {
			key = "（无指令）"
		}
		groups[key] = append(groups[key], record)
	}
	summary.ByCommand = make(map[string]StatsSummary, len(groups))
	for key, group := range groups {
		summary.ByCommand[key] = summarizeStats(group, false)
	}
	return summary
}

// percentile reads the p-th percentile from an ascending sorted slice.
func percentile(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	index := int(p * float64(len(sorted)-1))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// RecordMessageStats stores one completed handling record. The scope id is
// required; a negative duration is clamped so a clock hiccup cannot poison the
// aggregate.
func (s *Service) RecordMessageStats(record MessageStats) error {
	record.ScopeID = strings.TrimSpace(record.ScopeID)
	if record.ScopeID == "" {
		return fmt.Errorf("统计记录缺少 scope_id。")
	}
	record.RenderMS = clampMS(record.RenderMS)
	record.UploadMS = clampMS(record.UploadMS)
	record.SendMS = clampMS(record.SendMS)
	record.ServerMS = clampMS(record.ServerMS)
	if record.ReceivedAt.IsZero() {
		record.ReceivedAt = time.Now()
	}
	return s.store.InsertMessageStats(record)
}

func clampMS(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// PruneMessageStats drops records older than StatsRetention.
func (s *Service) PruneMessageStats(now time.Time) (int, error) {
	return s.store.PruneMessageStats(now.Add(-StatsRetention))
}

// MessageStatsSummary aggregates the records received at or after since.
func (s *Service) MessageStatsSummary(since time.Time, scopeID string) (StatsSummary, error) {
	records, err := s.store.ListMessageStats(since, scopeID)
	if err != nil {
		return StatsSummary{}, err
	}
	return summarizeStats(records, true), nil
}
