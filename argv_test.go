package main

import (
	"testing"
)

// argvParseTests is a map of test case => returned slice for the Tokenize
// function.
var argvParseTests = map[string][]string{
	"this is a test":           {"this", "is", "a", "test"},
	"this  is   a   test":      {"this", "is", "a", "test"},
	"this  is\na   test":       {"this", "is", "a", "test"},
	"this \"is a\"   test":     {"this", "is a", "test"},
	"this \"is\na\"   test":    {"this", "is\na", "test"},
	"this 'is a'   test":       {"this", "is a", "test"},
	"this 'is\na' \n\t  test":  {"this", "is\na", "test"},
	"this 'is \" a'   test":    {"this", "is \" a", "test"},
	"this \"is 'a\"   test":    {"this", "is 'a", "test"},
	"this \\\"is a\\\"   test": {"this", "\"is", "a\"", "test"},
	"this is\\\ta test":        {"this", "is\ta", "test"},
	"this \\is a test":         {"this", "\\is", "a", "test"},
	"this \\'is a test":        {"this", "'is", "a", "test"},
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestTokenize(t *testing.T) {
	for k, v := range argvParseTests {
		tokens, err := Tokenize(k)
		if err != nil {
			t.Errorf("An unexpected error occured: %s", err.Error())
			continue
		}
		t.Logf("%s => %s", k, tokens)
		if !equal(v, tokens) {
			t.Errorf("%t != computed: %t", v, tokens)
		}
	}
}

func TestTokenizeError(t *testing.T) {
	// These strings do not successfully parse due to missing quotes
	var badStrings = []string{
		"this is a \"test",
		"this is a 'test",
	}

	for _, v := range badStrings {
		tokens, err := Tokenize(v)
		if err == nil {
			t.Errorf("This string should return an error: \"%s\" but it returned a successful parsed slice %s", v, tokens)
		} else {
			t.Logf("Error from bad string returned as expected: %s", err.Error())
		}
	}
}
