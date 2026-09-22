package webhook

import (
	"strings"
	"testing"

	"github.com/mico-v/qqbot-course-schedule/internal/bot"
)

func TestPreviewText(t *testing.T) {
	if got := previewText("  hello\n\nworld  ", 120); got != "hello world" {
		t.Fatalf("previewText = %q", got)
	}
	long := strings.Repeat("字", 130)
	got := previewText(long, 120)
	if len([]rune(got)) != 121 || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated preview length = %d, suffix = %v", len([]rune(got)), strings.HasSuffix(got, "…"))
	}
}

func TestMessageTypeText(t *testing.T) {
	cases := map[int]string{0: "text", 3: "ark", 101: "parallel", 102: "forward", 103: "quote", 9: ""}
	for input, want := range cases {
		if got := messageTypeText(input); got != want {
			t.Fatalf("messageTypeText(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestLogInboundMessageDoesNotPanic(t *testing.T) {
	logInboundMessage(EventGroupMessage, 0, &bot.Message{Origin: bot.OriginGroup})
	logInboundMessage(EventC2CMessage, 102, &bot.Message{
		Origin:      bot.OriginPrivate,
		UserOpenID:  "U1",
		Username:    "小明",
		Content:     "hi",
		Attachments: []bot.Attachment{{Filename: "a.ics", ContentType: "file"}},
	})
}
