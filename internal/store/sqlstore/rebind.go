package sqlstore

import (
	"strconv"
	"strings"
)

func rebindDollar(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(query) + 16)

	inSingle := false
	inDouble := false
	arg := 1

	for i := 0; i < len(query); i++ {
		c := query[i]

		if c == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(c)
			continue
		}

		if !inSingle && !inDouble && c == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(arg))
			arg++
			continue
		}

		b.WriteByte(c)
	}

	return b.String()
}

