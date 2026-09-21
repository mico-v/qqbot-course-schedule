package schedule

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// DecodeICSBytes converts an uploaded file into UTF-8 text. Chinese school
// systems export GBK or UTF-16 surprisingly often, so the decoder normalizes
// the common encodings instead of rejecting them.
func DecodeICSBytes(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}): // UTF-8 BOM
		return string(data[3:])
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}): // UTF-16 LE BOM
		if decoded, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().Bytes(data[2:]); err == nil {
			return string(decoded)
		}
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}): // UTF-16 BE BOM
		if decoded, err := unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder().Bytes(data[2:]); err == nil {
			return string(decoded)
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
		return string(decoded)
	}
	return string(data)
}

// sniffICSContent describes a file that does not look like iCalendar, so the
// chat reply points at what the user actually sent.
func sniffICSContent(content string) string {
	trimmed := strings.TrimSpace(content)
	switch {
	case trimmed == "":
		return "（文件内容为空）"
	case strings.HasPrefix(trimmed, "PK\x03\x04"):
		return "（内容看起来是压缩包，请解压后发送 .ics 文件）"
	case strings.HasPrefix(trimmed, "\xD0\xCF\x11\xE0"):
		return "（内容看起来是 Excel 文件）"
	case strings.HasPrefix(trimmed, "<"):
		return "（内容看起来是 HTML/XML，请发送真正的 .ics 文件）"
	}
	preview := []rune(trimmed)
	if len(preview) > 60 {
		preview = preview[:60]
	}
	return fmt.Sprintf("（文件开头：%s）", string(preview))
}
