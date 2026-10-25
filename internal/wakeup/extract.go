package wakeup

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	shareCodeRe = regexp.MustCompile(
		`(?i)(?:分享口令|分享码|口令)\s*(?:为|是|[:：=])?\s*[「【\[\(（]?\s*([A-Za-z0-9][A-Za-z0-9_-]{1,127})`,
	)
	bracketCodeRe = regexp.MustCompile(
		`[「【\[\(（]\s*([A-Za-z0-9][A-Za-z0-9_-]{1,127})\s*[」】\]\)）]`,
	)
	urlTextRe       = regexp.MustCompile(`(?i)https?://[^\s<>"'」】）)]+`)
	codeCandidateRe = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9_-]{3,127}`)
	shareCodeNoise  = map[string]bool{"http": true, "https": true, "wakeup": true, "code": true, "share": true, "openid": true}
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
	if m := bracketCodeRe.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	if matchedURL := urlTextRe.FindString(text); matchedURL != "" {
		matchedURL = strings.TrimRight(matchedURL, ".,，。;；!?！？")
		if parsed, err := url.Parse(matchedURL); err == nil {
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
			if tail != "" && isCodeCandidate(tail) {
				return tail
			}
		}
	}
	if isCodeCandidate(text) {
		return text
	}
	var candidates []string
	for _, candidate := range codeCandidateRe.FindAllString(text, -1) {
		if shareCodeNoise[strings.ToLower(candidate)] || !isCodeCandidate(candidate) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

func isCodeCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 128 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}
