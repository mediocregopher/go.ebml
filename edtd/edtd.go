package edtd

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mediocregopher/go.ebml/varint"
)

type Type int

const (
	Int Type = iota
	Uint
	Float
	String
	Date
	Binary
	Container
)

type card int

const (
	zeroOrOnce card = iota
	zeroOrMore
	exactlyOnce
	oneOrMore
)

type (
	elementID  int64
	elementMap map[elementID]*tplElement
	typesMap   map[string]*tplElement
)

type tplElement struct {
	id   elementID
	typ  Type
	name string
	kids []tplElement
	def  []byte
	size uint64
	card
	ranges       *rangeParam
	mustMatchDef bool
}

// Edtd is generated from an edtd specification. It can be used to generate one
// or more Parsers, which will read in streams of ebml data and attempt to parse
// them based on this edtd
type Edtd struct {
	elements elementMap
	types    typesMap
}

var implicitElements = `
    EBML := 1a45dfa3 container [ card:+; ] {
      EBMLVersion := 4286 uint [ def:1; ]
      EBMLReadVersion := 42f7 uint [ def:1; ]
      EBMLMaxIDLength := 42f2 uint [ def:4; ]
      EBMLMaxSizeLength := 42f3 uint [ def:8; ]
      DocType := 4282 string [ range:32..126; ]
      DocTypeVersion := 4287 uint [ def:1; ]
      DocTypeReadVersion := 4285 uint [ def:1; ]
    }

    CRC32 := c3 container [ level:1..; card:*; ] {
      %children;
      CRC32Value := 42fe binary [ size:4; ]
    }

    Void  := ec binary [ level:1..; card:*; ]

// Closing curly brace is a hack to get parseElements to stop
}
`

var (
	defineTok    = token{alphaNum, "define"}
	elementsTok  = token{alphaNum, "elements"}
	headerTok    = token{alphaNum, "header"}
	typesTok     = token{alphaNum, "types"}
	declareTok   = token{alphaNum, "declare"}
	childrenTok  = token{alphaNum, "children"}
	assignTok    = token{control, ":="}
	openCurlyTok = token{control, "{"}
	colonTok     = token{control, ":"}
	semiColonTok = token{control, ";"}
	commaTok     = token{control, ","}
)

// Pulls the next token from the lexer and checks if it matches any of the
// tokens, returning the one it matches or an error
func expect(lex *lexer, tok ...*token) (*token, error) {
	nextTok := lex.next()
	for i := range tok {
		if *nextTok == *tok[i] {
			return tok[i], nil
		}
	}
	return nil, fmt.Errorf("expected one of %v but found '%s'", tok, nextTok)
}

// Pulls the next token from the lexer and checks if it is of the given type,
// returning it if it is or an error
func expectType(lex *lexer, typ ...tokentyp) (*token, error) {
	nextTok := lex.next()
	for i := range typ {
		if nextTok.typ == typ[i] {
			return nextTok, nil
		}
	}
	return nil, fmt.Errorf("unexpected token '%s' found", nextTok)
}

// Will read from the io.Reader until EOF, creating an internal structure for
// understanding ebml streams which conform to the edtd read in
func NewEdtd(r io.Reader) (*Edtd, error) {

	lex := newLexer(r)
	m := elementMap{}
	t := typesMap{}

	implicitBuf := bytes.NewBufferString(implicitElements)
	if _, err := parseElements(newLexer(implicitBuf), m, t, false); err != nil {
		return nil, err
	}

	for {
		defdecTok := lex.next()
		if defdecTok.typ == eof {
			return &Edtd{m, t}, nil
		} else if defdecTok.val != "declare" && defdecTok.val != "define" {
			return nil, fmt.Errorf("unexpected token '%s' found", defdecTok)
		}

		defWhat, err := expect(lex, &elementsTok, &headerTok, &typesTok)
		if err != nil {
			return nil, err
		}

		if _, err := expect(lex, &openCurlyTok); err != nil {
			return nil, err
		}

		switch defWhat.val {
		case "elements":
			if _, err := parseElements(lex, m, t, false); err != nil {
				return nil, err
			}

		case "header":
			if err := parseHeader(lex, m); err != nil {
				return nil, err
			}

		case "types":
			if err := parseTypes(lex, t); err != nil {
				return nil, err
			}

		default:

		}
	}
}

// Basically the same as parseElements, but we read into the typesMap which
// indexes by the name instead of the id
func parseTypes(lex *lexer, t typesMap) error {
	fakem := elementMap{}
	elems, err := parseElements(lex, fakem, t, true)
	if err != nil {
		return err
	}

	for i := range elems {
		name := strings.ToLower(elems[i].name)
		t[name] = &elems[i]
	}
	return nil
}

// dontExpectId is used by parseTypes, which parses exactly like parseElements
// except that there are no ids
func parseElements(
	lex *lexer, m elementMap, t typesMap, dontExpectId bool,
) (
	[]tplElement, error,
) {
	elems := make([]tplElement, 0, 8)
	for {
		elem, err, done := parseElement(lex, m, t, dontExpectId)
		if err != nil {
			return nil, err
		} else if done {
			return elems, nil
		}
		elems = append(elems, elem)
	}
}

// Parses a single element out and returns it. Since this is expected to run
// within a container to parse out the elements within a container, it also
// handles running into a closing curly brace (the end of the container). In
// thise case it returns the third argument as true and doesn't parse anything
// out
func parseElement(
	lex *lexer, m elementMap, t typesMap, dontExpectId bool,
) (
	tplElement, error, bool,
) {
	nameTok := lex.next()
	if err := nameTok.asError(); err != nil {
		return tplElement{}, err, false
	} else if nameTok.typ == control && nameTok.val == "}" {
		return tplElement{}, nil, true
	} else if nameTok.val == "%" {
		if _, err = expect(lex, &childrenTok); err != nil {
			return tplElement{}, err, false
		}
		if _, err = expect(lex, &semiColonTok); err != nil {
			return tplElement{}, err, false
		}
		return parseElement(lex, m, t, dontExpectId)
	} else if nameTok.typ != alphaNum {
		return tplElement{}, fmt.Errorf("unexpected '%s' found", nameTok), false
	}

	if _, err := expect(lex, &assignTok); err != nil {
		return tplElement{}, err, false
	}

	var id elementID
	if dontExpectId {
		id = 0
	} else {
		idTok, err := expectType(lex, alphaNum)
		if err != nil {
			return tplElement{}, err, false
		}

		id, err = strToID(idTok.val)
		if err != nil {
			return tplElement{}, err, false
		}
	}

	typTok, err := expectType(lex, alphaNum)
	if err != nil {
		return tplElement{}, err, false
	}

	var elem tplElement
	if typ, ok := strToType(typTok.val); ok {
		elem = tplElement{
			id:   id,
			typ:  typ,
			name: nameTok.val,
		}
	} else if typTpl, ok := t[strings.ToLower(typTok.val)]; ok {
		elem = *typTpl
		elem.id = id
		elem.name = nameTok.val
	}

	m[elem.id] = &elem

	controlTok, err := expectType(lex, control)
	if err != nil {
		return elem, err, false
	}

	// This gets a bit hairy.
	// * If the control character is a ; the element is done
	// * If it is a [ then there are parameters to read
	// * There are then child elements after a { if-and-only-if the element is a
	//   container
	if controlTok.val == ";" {
		return elem, nil, false
	} else if controlTok.val == "[" {
		if err := parseParams(lex, &elem); err != nil {
			return elem, err, false
		}
	} else if elem.typ != Container {
		return elem, fmt.Errorf("unexpected token '%s'", controlTok.val), false
	}

	if elem.typ != Container {
		return elem, nil, false
	}

	// if controlTok was just a [ then we expect there to be a { afterwards
	if elem.typ == Container && controlTok.val != "{" {
		if controlTok, err = expectType(lex, control); err != nil {
			return elem, err, false
		}
		if controlTok.val != "{" {
			return elem, fmt.Errorf("unexpected token '%s'", controlTok.val), false
		}
	}

	kids, err := parseElements(lex, m, t, dontExpectId)
	if err != nil {
		return elem, err, false
	}
	elem.kids = kids

	if elem.typ == Container && elem.kids == nil {
		elem.kids = make([]tplElement, 0)
	}

	return elem, nil, false
}

func strToID(s string) (elementID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return 0, err
	}
	i, err := varint.VarInt(b)
	return elementID(i), err
}

func strToType(s string) (Type, bool) {
	switch strings.ToLower(s) {
	case "int":
		return Int, true
	case "uint":
		return Uint, true
	case "float":
		return Float, true
	case "string":
		return String, true
	case "date":
		return Date, true
	case "binary":
		return Binary, true
	case "container":
		return Container, true
	default:
		return 0, false
		//return 0, fmt.Errorf("unknown type '%s'", s)
	}
}

func parseParams(lex *lexer, elem *tplElement) error {
	for {
		err, done := parseParam(lex, elem)
		if err != nil {
			return err
		} else if done {
			return nil
		}
	}
}

// Reads a single parameter for an element and parses it, modifying the element
// as needed. Returns an error, and boolean which will be true if the closing
// bracked has been reached
func parseParam(lex *lexer, elem *tplElement) (error, bool) {
	pnameTok := lex.next()
	if err := pnameTok.asError(); err != nil {
		return err, false
	} else if pnameTok.typ == control && pnameTok.val == "]" {
		return nil, true
	} else if pnameTok.typ != alphaNum {
		return fmt.Errorf("Unknown param field '%s'", pnameTok.val), false
	}

	if _, err := expect(lex, &colonTok); err != nil {
		return err, false
	}

	pvalTok, err := expectType(lex, alphaNum, quotedString, control)
	if err != nil {
		return err, false
	}

	switch pnameTok.val {
	case "card":
		if err := parseCardParam(lex, elem, pvalTok); err != nil {
			return err, false
		}
		if _, err := expect(lex, &semiColonTok); err != nil {
			return err, false
		}
	case "def":
		if err := parseDefParam(elem, pvalTok); err != nil {
			return err, false
		}
		if _, err := expect(lex, &semiColonTok); err != nil {
			return err, false
		}
	case "size":
		if err := parseSizeParam(elem, pvalTok); err != nil {
			return err, false
		}
		if _, err := expect(lex, &semiColonTok); err != nil {
			return err, false
		}
	case "range":
		// Ranges can have multiple values, each separated by a comma
		rangeToks := append(make([]*token, 0, 2), pvalTok)
		for {
			controlTok, err := expect(lex, &semiColonTok, &commaTok)
			if err != nil {
				return err, false
			}
			if controlTok.val == ";" {
				break
			}
			pvalTok, err = expectType(lex, alphaNum)
			if err != nil {
				return err, false
			}
			rangeToks = append(rangeToks, pvalTok)
		}
		rangeParams, err := parseRangeParams(elem.typ, rangeToks)
		if err != nil {
			return err, false
		}
		elem.ranges = rangeParams
	default:
		if _, err = expect(lex, &semiColonTok); err != nil {
			return err, false
		}
	}

	return nil, false
}

func parseCardParam(lex *lexer, elem *tplElement, pvalTok *token) error {
	switch pvalTok.val {
	case "*":
		elem.card = zeroOrMore
	case "?":
		elem.card = zeroOrOnce
	case "1":
		elem.card = exactlyOnce
	case "+":
		elem.card = oneOrMore
	default:
		return fmt.Errorf("unknown cardinality '%s'", pvalTok.val)
	}
	return nil
}

func parseDefParam(elem *tplElement, pvalTok *token) error {
	switch elem.typ {
	case Int:
		i, err := strconv.ParseInt(pvalTok.val, 10, 64)
		if err != nil {
			return err
		}
		return setDefData(elem, &i)
	case Uint:
		i, err := strconv.ParseUint(pvalTok.val, 10, 64)
		if err != nil {
			return err
		}
		return setDefData(elem, &i)
	case Float:
		f, err := strconv.ParseFloat(pvalTok.val, 64)
		if err != nil {
			return err
		}
		return setDefData(elem, &f)
	case String, Binary:
		if pvalTok.val[:2] == "0x" {
			s, err := hex.DecodeString(pvalTok.val[2:])
			if err != nil {
				return err
			}
			elem.def = []byte(s)
			return nil
		}
		if pvalTok.typ != quotedString {
			return fmt.Errorf("quoted string expected")
		}
		s, err := strconv.Unquote(pvalTok.val)
		if err != nil {
			return err
		}
		elem.def = []byte(s)
		return nil
	case Date:
		return fmt.Errorf("Default date not supported yet")
	default:
		return fmt.Errorf("Found default on unsupported type")
	}
}

func setDefData(elem *tplElement, d interface{}) error {
	if b, err := defDataBytes(d); err != nil {
		return err
	} else {
		elem.def = b
		return nil
	}
}

func parseSizeParam(elem *tplElement, pvalTok *token) error {
	i, err := strconv.ParseUint(pvalTok.val, 10, 64)
	if err != nil {
		return err
	}
	elem.size = i
	return nil
}

func defDataBytes(d interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 8))
	if err := binary.Write(buf, binary.BigEndian, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
