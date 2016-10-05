package main

import (
	"fmt"
	"unicode"
)

// State 0: No quote
// State 1: backslash
// State 2: In Quote

// Tokenize splits string s around each instance of one or more white space
// characters.  Unlike strings.Fields() it supports the use of shell-like
// quoting or tokenization to allow the returned substrings to contain white
// space.
func Tokenize(s string) ([]string, error) {
	var (
		result []string
		token  []rune
		state  int
		quote  rune
	)

	chunk := func() {
		if len(token) > 0 {
			result = append(result, string(token))
			token = nil // zero length slice
		}
	}

	for _, c := range s {
		switch state {
		case 0:
			switch {
			case unicode.IsSpace(c):
				chunk()
			case c == '\\':
				state = 1
			case c == '\'' || c == '"':
				state = 2
				quote = c
			default:
				token = append(token, c)
			}
		case 1:
			switch {
			case unicode.IsSpace(c):
				fallthrough
			case c == '\'':
				fallthrough
			case c == '"':
				token = append(token, c)
			default:
				token = append(token, '\\')
				token = append(token, c)
			}
			if quote == '\x00' {
				state = 0
			} else {
				state = 2
			}
		case 2:
			switch c {
			case '\\':
				state = 1
			case quote:
				chunk()
				quote = '\x00'
				state = 0
			default:
				token = append(token, c)
			}
		}
	}

	// End of String
	switch state {
	case 1:
		token = append(token, '\\')
	case 2:
		return nil, fmt.Errorf("Missing closing quote")
	}

	chunk()
	return result, nil
}
