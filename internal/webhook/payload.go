package webhook

import "encoding/json"

// Event types handled by the bot.
const (
	EventGroupAtMessage = "GROUP_AT_MESSAGE_CREATE"
	EventGroupMessage   = "GROUP_MESSAGE_CREATE"
	EventC2CMessage     = "C2C_MESSAGE_CREATE"
	EventInteraction    = "INTERACTION_CREATE"
	EventGroupMsgRecv   = "GROUP_MSG_RECEIVE"
	EventC2CMsgRecv     = "C2C_MSG_RECEIVE"
)

// Payload is the common webhook envelope.
type Payload struct {
	ID string          `json:"id"`
	Op int             `json:"op"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
}

// ValidationData is the d field of an op=13 request.
type ValidationData struct {
	PlainToken string `json:"plain_token"`
	EventTS    string `json:"event_ts"`
}

// MessageData is the d field of a message event
// (GROUP_AT_MESSAGE_CREATE / GROUP_MESSAGE_CREATE / C2C_MESSAGE_CREATE).
type MessageData struct {
	ID          string       `json:"id"`
	Content     string       `json:"content"`
	GroupOpenID string       `json:"group_openid"`
	Author      Author       `json:"author"`
	Attachments []Attachment `json:"attachments"`
	Mentions    []Author     `json:"mentions"`
	Timestamp   string       `json:"timestamp"`
	MsgSeq      int          `json:"msg_seq"`
}

// Author is the message sender in a group or single chat.
type Author struct {
	ID           string `json:"id"`
	UserOpenID   string `json:"user_openid"`
	MemberOpenID string `json:"member_openid"`
	UnionOpenID  string `json:"union_openid"`
	Username     string `json:"username"`
	MemberRole   string `json:"member_role"`
	Bot          bool   `json:"bot"`
}

// Attachment is a file/image/voice/video carried by a message.
type Attachment struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// BroadcastData is the d field of GROUP_MSG_RECEIVE / C2C_MSG_RECEIVE.
type BroadcastData struct {
	GroupOpenID    string `json:"group_openid"`
	OpenID         string `json:"openid"`
	OPMemberOpenID string `json:"op_member_openid"`
	Timestamp      int64  `json:"timestamp"`
}

// InteractionData is the d field of INTERACTION_CREATE.
type InteractionData struct {
	ID                string `json:"id"`
	Type              int    `json:"type"`
	Scene             string `json:"scene"`
	GroupOpenID       string `json:"group_openid"`
	GroupMemberOpenID string `json:"group_member_openid"`
	UserOpenID        string `json:"user_openid"`
	Data              struct {
		Resolved struct {
			ButtonID   string `json:"button_id"`
			ButtonData string `json:"button_data"`
		} `json:"resolved"`
	} `json:"data"`
}
