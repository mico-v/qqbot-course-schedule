package wakeup

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	shareCodeRe  = regexp.MustCompile(`分享口令为\s*[「【\[]([^」】\]]+)[」】\]]`)
	shareAliasRe = regexp.MustCompile(`分享码[为:：]\s*[「【\[]?([^」】\]\s]+)`)
)

// ExtractShareCode pulls a WakeUp share code out of a share message, a URL or a
// bare code. The official message reads:
//
//	……打开App……分享口令为「CODE」
//
// URLs are accepted for convenience (?code=/…/CODE) even though the app itself
// only shares plain text.
func ExtractShareCode(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if m := shareCodeRe.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := shareAliasRe.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	if strings.Contains(text, "://") {
		if parsed, err := url.Parse(text); err == nil {
			query := parsed.Query()
			for _, key := range []string{"code", "shareCode", "share_code", "c"} {
				if value := strings.TrimSpace(query.Get(key)); value != "" {
					return value
				}
			}
			tail := strings.Trim(parsed.Path, "/")
			if idx := strings.LastIndex(tail, "/"); idx >= 0 {
				tail = tail[idx+1:]
			}
			if tail != "" {
				return tail
			}
		}
	}
	return text
}
