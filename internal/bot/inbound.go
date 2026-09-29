package bot

import (
	"context"
	"time"

	"github.com/mico-v/qqbot-course-schedule/internal/timing"
)

// Origin is where a message came from.
type Origin string

const (
	OriginGroup   Origin = "group"
	OriginPrivate Origin = "private"
)

// Inbound is one received message: the immutable facts the router and the
// command handlers need. It carries no way to answer, so a handler cannot
// accidentally close over sending ability while handling received data.
type Inbound struct {
	Origin      Origin
	GroupOpenID string
	UserOpenID  string
	MsgID       string
	EventID     string
	Content     string
	// Args is set by Handler.Dispatch: the text after the matched prefix.
	Args string
	// Command is set by Handler.Dispatch: the matched command prefix.
	Command     string
	Username    string
	MemberRole  string
	IsBot       bool
	Attachments []Attachment
	Mentions    []Mention

	// receivedCtx carries the webhook arrival stamp used for timing. It is nil
	// for messages built by tests, where ReceivedAt falls back to now.
	receivedCtx context.Context
}

// Mention is one @ user carried by an inbound message.
type Mention struct {
	ID   string
	Name string
}

// Attachment is one file carried by an inbound message.
type Attachment struct {
	URL         string
	Filename    string
	ContentType string
	Size        int64
}

// IsAdmin reports whether the sender is a group owner or administrator.
func (in *Inbound) IsAdmin() bool {
	if in == nil {
		return false
	}
	return in.MemberRole == "admin" || in.MemberRole == "owner"
}

// NewInbound builds one received message and stamps it with the webhook
// acceptance time carried by ctx, so timing later measures from that origin.
func NewInbound(ctx context.Context, origin Origin, groupOpenID, userOpenID, msgID, eventID, content, username, memberRole string, attachments []Attachment, mentions []Mention) *Inbound {
	return (&Inbound{
		Origin:      origin,
		GroupOpenID: groupOpenID,
		UserOpenID:  userOpenID,
		MsgID:       msgID,
		EventID:     eventID,
		Content:     content,
		Username:    username,
		MemberRole:  memberRole,
		Attachments: attachments,
		Mentions:    mentions,
	}).WithReceived(ctx)
}

// WithReceived stamps the inbound with the moment the webhook was accepted, so
// every later timing measurement shares one origin. It returns the receiver so
// construction reads as one expression.
func (in *Inbound) WithReceived(ctx context.Context) *Inbound {
	if in != nil {
		in.receivedCtx = ctx
	}
	return in
}

// ReceivedAt returns when the webhook for this message was accepted. Messages
// built without a stamp report the current time, which makes their measured
// duration zero rather than an absurd value.
func (in *Inbound) ReceivedAt() time.Time {
	if in != nil {
		if at, ok := timing.Received(in.receivedCtx); ok {
			return at
		}
	}
	return time.Now()
}
