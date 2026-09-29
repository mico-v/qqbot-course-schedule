package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCardKeyboards(t *testing.T) {
	dayKeyboard := cardKeyboardForDay("2026-09-17", "2026-09-17", "U1")
	buttons := dayKeyboard.Content.Rows[0].Buttons
	if len(buttons) != 3 {
		t.Fatalf("buttons = %d", len(buttons))
	}
	if buttons[0].Action.Data != "day:2026-09-16" || buttons[2].Action.Data != "day:2026-09-18" {
		t.Fatalf("day data = %q %q", buttons[0].Action.Data, buttons[2].Action.Data)
	}
	if buttons[0].Action.Permission == nil || len(buttons[0].Action.Permission.SpecifyUserIDs) != 1 ||
		buttons[0].Action.Permission.SpecifyUserIDs[0] != "U1" {
		t.Fatalf("permission = %+v", buttons[0].Action.Permission)
	}
	rankKeyboard := cardKeyboardForRank("U1")
	rankData := rankKeyboard.Content.Rows[0].Buttons[0].Action.Data
	if rankData != "rank:thisweek" {
		t.Fatalf("rank data = %q", rankData)
	}
}

func TestHandleCallbackRendersRequestedDay(t *testing.T) {
	fake := newFakeQQ()
	apiServer := httptest.NewServer(fake.handler())
	t.Cleanup(apiServer.Close)
	icsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testICS))
	}))
	t.Cleanup(icsServer.Close)

	env, base := newTestEnv(t, fake, apiServer.URL)
	handler := NewDefaultHandler(env)
	ctx := context.Background()
	freshImport := freshMessage(base)
	if err := env.ImportICS(ctx, freshImport, NewReplier(freshImport, env.Client), Attachment{URL: icsServer.URL + "/s.ics", Filename: "s.ics"}); err != nil {
		t.Fatal(err)
	}
	_ = waitMessage(t, fake)

	handler.HandleCallback(ctx, &Callback{
		Data:        "day:2026-09-18",
		EventID:     "INT-9",
		GroupOpenID: "GROUP",
		UserOpenID:  "U1",
	})
	card := waitMessage(t, fake)
	if card["event_id"] != "INT-9" {
		t.Fatalf("callback reply must use event_id: %+v", card)
	}
	if msgType, _ := card["msg_type"].(float64); msgType != 7 {
		t.Fatalf("callback card = %+v", card)
	}
}
