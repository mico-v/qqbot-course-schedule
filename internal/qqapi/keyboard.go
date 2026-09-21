package qqapi

// Keyboard is the inline keyboard attached below a markdown message.
// The platform currently gates custom keyboards behind an invite (内邀).
type Keyboard struct {
	Content KeyboardContent `json:"content"`
}

// KeyboardContent holds up to 5 rows of up to 5 buttons.
type KeyboardContent struct {
	Rows []KeyboardRow `json:"rows"`
}

// KeyboardRow is one button row.
type KeyboardRow struct {
	Buttons []KeyboardButton `json:"buttons"`
}

// KeyboardButton is one inline button.
type KeyboardButton struct {
	ID         string           `json:"id,omitempty"`
	RenderData ButtonRenderData `json:"render_data"`
	Action     ButtonAction     `json:"action"`
	GroupID    string           `json:"group_id,omitempty"`
}

// ButtonRenderData controls the button label and style.
type ButtonRenderData struct {
	Label        string `json:"label"`
	VisitedLabel string `json:"visited_label"`
	Style        int    `json:"style"`
}

// ButtonAction describes what happens when the button is pressed.
type ButtonAction struct {
	Type          int               `json:"type"`
	Data          string            `json:"data,omitempty"`
	Permission    *ButtonPermission `json:"permission,omitempty"`
	UnsupportTips string            `json:"unsupport_tips,omitempty"`
}

// ButtonPermission restricts who may press the button.
type ButtonPermission struct {
	Type           int      `json:"type"`
	SpecifyUserIDs []string `json:"specify_user_ids,omitempty"`
}

// Button styles.
const (
	ButtonStyleGray = 0
	ButtonStyleBlue = 1
)

// Button action types.
const (
	ButtonActionLink     = 0
	ButtonActionCallback = 1
	ButtonActionCommand  = 2
)

// Button permission types.
const (
	ButtonPermissionSomeUser = 0
	ButtonPermissionAdmin    = 1
	ButtonPermissionAll      = 2
)
