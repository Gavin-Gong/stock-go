// Package gbk decodes Windows-936 text with the pinned source TextDecoder mappings.
package gbk

import (
	"golang.org/x/text/encoding/simplifiedchinese"
	"strings"
)

// Decode preserves Windows user-defined characters missing from the standard Go table.
// Invalid byte sequences produce the Unicode replacement character.
func Decode(data []byte) (string, error) {
	var out strings.Builder
	start := 0
	flush := func(end int) error {
		if start == end {
			return nil
		}
		s, e := simplifiedchinese.GBK.NewDecoder().Bytes(data[start:end])
		if e == nil {
			out.Write(s)
		}
		return e
	}
	for i := 0; i < len(data); {
		size := 1
		key := uint16(data[i])
		if data[i] >= 0x81 && data[i] <= 0xfe && i+1 < len(data) && data[i+1] >= 0x40 && data[i+1] <= 0xfe && data[i+1] != 0x7f {
			size = 2
			key = key<<8 | uint16(data[i+1])
		}
		if replacement, ok := overrides[key]; ok {
			if e := flush(i); e != nil {
				return "", e
			}
			out.WriteString(replacement)
			start = i + size
		}
		i += size
	}
	if e := flush(len(data)); e != nil {
		return "", e
	}
	return out.String(), nil
}
