package edtd

import (
	"bytes"
	. "testing"
)

// I'm being lazy and just doing a single giant fucking string. yolo
func Testlexer(t *T) {
	testStr := `
		define types {
			bool := uint [ range:0..1; ]
			ascii := string [ range:32..126; ]
		}
		define elements {
			
			// Snip... farther down the list
			TimecodeScale := 2ad7b1 uint [ def:1000000; ]
			Duration := 4489 float [ range:>0.0; ]

			// Snip
			Language := 22b59c string [ def:"eng"; range:32..126; ]
	`

	output := []token{
		{alphaNum, "define"},
		{alphaNum, "types"},
		{control,  "{"},

		{alphaNum, "bool"},
		{control, ":="},
		{alphaNum, "uint"},
		{control, "["},
		{alphaNum, "range"},
		{control, ":"},
		{alphaNum, "0..1"},
		{control, ";"},
		{control, "]"},

		{alphaNum, "ascii"},
		{control, ":="},
		{alphaNum, "string"},
		{control, "["},
		{alphaNum, "range"},
		{control, ":"},
		{alphaNum, "32..126"},
		{control, ";"},
		{control, "]"},
		
		{control, "}"},

		{alphaNum, "define"},
		{alphaNum, "elements"},
		{control, "{"},

		{alphaNum, "TimecodeScale"},
		{control, ":="},
		{alphaNum, "2ad7b1"},
		{alphaNum, "uint"},
		{control, "["},
		{alphaNum, "def"},
		{control, ":"},
		{alphaNum, "1000000"},
		{control, ";"},
		{control, "]"},

		{alphaNum, "Duration"},
		{control, ":="},
		{alphaNum, "4489"},
		{alphaNum, "float"},
		{control, "["},
		{alphaNum, "range"},
		{control, ":"},
		{control, ">"},
		{alphaNum, "0.0"},
		{control, ";"},
		{control, "]"},

		{alphaNum, "Language"},
		{control, ":="},
		{alphaNum, "22b59c"},
		{alphaNum, "string"},
		{control, "["},
		{alphaNum, "def"},
		{control, ":"},
		{quotedString, "\"eng\""},
		{control, ";"},
		{alphaNum, "range"},
		{control, ":"},
		{alphaNum, "32..126"},
		{control, ";"},
		{control, "]"},
		{eof, ""},
	}

	buf := bytes.NewBufferString(testStr)
	l := newLexer(buf)

	for i := range output {
		tok := l.next()
		t.Logf("Checking for %#v", output[i])
		if *tok != output[i] {
			t.Fatalf("Found %#v instead of %#v", *tok, output[i])
		}
	}
}