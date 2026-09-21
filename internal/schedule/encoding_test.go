package schedule

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

func TestDecodeICSBytesEncodings(t *testing.T) {
	text := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:u1\r\nSUMMARY:高等数学\r\nDTSTART:20260901T080000\r\nDTEND:20260901T093000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	// UTF-8 BOM.
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte(text)...)
	if got := DecodeICSBytes(withBOM); !strings.HasPrefix(got, "BEGIN:VCALENDAR") {
		t.Errorf("UTF-8 BOM decode = %q", got)
	}

	// UTF-16 LE with BOM.
	utf16Bytes, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if got := DecodeICSBytes(utf16Bytes); !strings.Contains(got, "高等数学") {
		t.Errorf("UTF-16 decode = %q", got)
	}

	// GBK.
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	decoded := DecodeICSBytes(gbkBytes)
	if !strings.Contains(decoded, "高等数学") {
		t.Errorf("GBK decode = %q", decoded)
	}

	events, err := ParseICSEvents(string(gbkBytes))
	if err != nil {
		t.Fatalf("ParseICSEvents(GBK): %v", err)
	}
	if len(events) != 1 || events[0]["SUMMARY"] != "高等数学" {
		t.Errorf("GBK events = %+v", events)
	}
}

func TestParseICSEventsErrorHints(t *testing.T) {
	cases := []struct {
		content string
		hint    string
	}{
		{"PK\x03\x04zipdata", "压缩包"},
		{"<!DOCTYPE html><html></html>", "HTML"},
		{"\xD0\xCF\x11\xE0excel", "Excel"},
		{"随便一段文字", "文件开头"},
	}
	for _, testCase := range cases {
		_, err := ParseICSEvents(testCase.content)
		if err == nil || !strings.Contains(err.Error(), testCase.hint) {
			t.Errorf("ParseICSEvents(%q) err = %v, want hint %q", testCase.content, err, testCase.hint)
		}
	}
}
