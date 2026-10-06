package schedule

// KV storage coordinates are owned by the schedule package so adapters and
// callers do not need to duplicate persistence naming.
const (
	// KVScopeGlobal is the scope used by bot-wide and cross-scope KV records.
	KVScopeGlobal = "global"
	// KVNamespaceSettings stores the bot-wide runtime settings.
	KVNamespaceSettings = "settings"
	// KVNamespaceSeen stores members observed through incoming messages.
	KVNamespaceSeen = "seen"
	// KVNamespacePanel stores command panel state managed by the bot.
	KVNamespacePanel = "panel"
	// KVNamespaceEditLinks stores hashed schedule edit-link tokens.
	KVNamespaceEditLinks = "edit_links"
)
