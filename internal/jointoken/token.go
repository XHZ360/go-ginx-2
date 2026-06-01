package jointoken

import (
	"strings"
	"unicode"
)

func Normalize(token string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, token)
}
