// Package bot holds the command router and the per-message context.
package bot

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mico-v/qqbot-course-schedule/internal/qqapi"
)

// Origin is where a message came from.
type Origin string

const (
	OriginGroup   Origin = "group"
	OriginPrivate Origin = "private"
)

// maxPassiveReplies is the platform limit for replies to one msg_id.
const maxPassiveReplies = 5

// ErrPassiveLimit means the 5-reply window for the message is exhausted.
var ErrPassiveLimit = errors.New("被动回复次数已达上限(5)")

// Message is one inbound message and the state needed to answer it.
type Message struct {
	Origin      Origin
	GroupOpenID string
	UserOpenID  string
	MsgID       string
	Content     string
	Args        string
	Username    string
	MemberRole  string
	IsBot       bool
	Attachments []Attachment
	Mentions    []Mention

	Client *qqapi.Client

	seqMu sync.Mutex
	seq   int
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
func (m *Message) IsAdmin() bool {
	return m.MemberRole == "admin" || m.MemberRole == "owner"
}

// Reply sends a passive text reply and consumes one of the five reply slots.
func (m *Message) Reply(ctx context.Context, text string) error {
	seq, err := m.nextSeq()
	if err != nil {
		return err
	}
	switch m.Origin {
	case OriginGroup:
		if m.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		_, err = m.Client.SendGroupText(ctx, m.GroupOpenID, text, m.MsgID, seq)
	case OriginPrivate:
		if m.UserOpenID == "" {
			return errors.New("单聊消息缺少 user_openid")
		}
		_, err = m.Client.SendC2CText(ctx, m.UserOpenID, text, m.MsgID, seq)
	default:
		return fmt.Errorf("未知消息来源 %q", m.Origin)
	}
	return err
}

// ReplyImage sends a passive image message from a public URL.
func (m *Message) ReplyImage(ctx context.Context, imageURL string) error {
	seq, err := m.nextSeq()
	if err != nil {
		return err
	}
	switch m.Origin {
	case OriginGroup:
		if m.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		return m.Client.SendGroupImage(ctx, m.GroupOpenID, imageURL, m.MsgID, seq)
	case OriginPrivate:
		if m.UserOpenID == "" {
			return errors.New("单聊消息缺少 user_openid")
		}
		return m.Client.SendC2CImage(ctx, m.UserOpenID, imageURL, m.MsgID, seq)
	default:
		return fmt.Errorf("未知消息来源 %q", m.Origin)
	}
}

// Push sends a proactive text message with no msg_id.
func (m *Message) Push(ctx context.Context, text string) error {
	switch m.Origin {
	case OriginGroup:
		_, err := m.Client.SendGroupText(ctx, m.GroupOpenID, text, "", 0)
		return err
	case OriginPrivate:
		_, err := m.Client.SendC2CText(ctx, m.UserOpenID, text, "", 0)
		return err
	default:
		return fmt.Errorf("未知消息来源 %q", m.Origin)
	}
}

func (m *Message) nextSeq() (int, error) {
	m.seqMu.Lock()
	defer m.seqMu.Unlock()
	if m.seq >= maxPassiveReplies {
		return 0, ErrPassiveLimit
	}
	m.seq++
	return m.seq, nil
}
