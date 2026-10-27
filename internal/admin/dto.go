package admin

// ScopeMemberSummary is one member row in the admin scope list.
type ScopeMemberSummary struct {
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	EventCount int    `json:"event_count"`
	Revision   int64  `json:"revision"`
}

// ScopeSummary is one group with its scheduled members.
type ScopeSummary struct {
	ScopeID string               `json:"scope_id"`
	Members []ScopeMemberSummary `json:"members"`
}

// WebEvent is one event as the admin page sees it.
type WebEvent struct {
	ID          int    `json:"id"`
	UID         string `json:"uid"`
	Course      string `json:"course"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	RRule       string `json:"rrule"`
}

// PageSchedule is the admin page payload for one member.
type PageSchedule struct {
	ScopeID  string     `json:"scope_id"`
	UserID   string     `json:"user_id"`
	Name     string     `json:"name"`
	QQ       string     `json:"qq,omitempty"`
	Revision int64      `json:"revision"`
	Events   []WebEvent `json:"events"`
}

// WebEventInput is one editable course row.
type WebEventInput struct {
	ID          int    `json:"id"`
	UID         string `json:"uid"`
	Course      string `json:"course"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location"`
	RRule       string `json:"rrule"`
	Description string `json:"description"`
}

// SavePagePayload is the revision-bearing admin save command.
type SavePagePayload struct {
	ScopeID  string          `json:"scope_id"`
	UserID   string          `json:"user_id"`
	Revision *int64          `json:"revision"`
	Name     *string         `json:"name"`
	QQ       *string         `json:"qq"`
	Events   []WebEventInput `json:"events"`
}

// SavePageResult is returned after a schedule save.
type SavePageResult struct {
	ScopeID    string `json:"scope_id"`
	UserID     string `json:"user_id"`
	Name       string `json:"name"`
	Revision   int64  `json:"revision"`
	EventCount int    `json:"event_count"`
}

// PublicPageSchedule is the token-scoped editor payload.
type PublicPageSchedule struct {
	Name      string     `json:"name"`
	QQ        string     `json:"qq,omitempty"`
	Revision  int64      `json:"revision"`
	ExpiresAt string     `json:"expires_at"`
	Events    []WebEvent `json:"events"`
}

// PublicSavePayload edits only the fields exposed by a public token.
type PublicSavePayload struct {
	Revision *int64          `json:"revision"`
	Name     *string         `json:"name"`
	QQ       *string         `json:"qq"`
	Events   []WebEventInput `json:"events"`
}

// PublicSaveResult is the public save response.
type PublicSaveResult struct {
	Name       string `json:"name"`
	Revision   int64  `json:"revision"`
	EventCount int    `json:"event_count"`
}

// NewMember is one pending member that can receive an empty schedule.
type NewMember struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// WebDayOverride is one day marker in the admin UI.
type WebDayOverride struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Day       string `json:"day"`
	Kind      string `json:"kind"`
	SourceDay string `json:"source_day,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}
