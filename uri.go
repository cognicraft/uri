// Copyright 2013 Joshua Tacoma. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package uritemplates is a level 4 implementation of RFC 6570 (URI
// Template, http://tools.ietf.org/html/rfc6570).
//
// To use uri, parse a template string and expand it with a value
// map:
//
//	template, _ := uri.Parse("https://api.github.com/repos{/user,repo}")
//	values := make(map[string]interface{})
//	values["user"] = "jtacoma"
//	values["repo"] = "uritemplates"
//	expanded, _ := template.Expand(values)
//	fmt.Printf(expanded)
//
package uri

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
	unreserved = regexp.MustCompile("[^A-Za-z0-9\\-._~]")
	reserved   = regexp.MustCompile("[^A-Za-z0-9\\-._~:/?#[\\]@!$&'()*+,;=]")
	validname  = regexp.MustCompile("^([A-Za-z0-9_\\.]|%[0-9A-Fa-f][0-9A-Fa-f])+$")
	hex        = []byte("0123456789ABCDEF")
)

func pctEncode(src []byte) []byte {
	dst := make([]byte, len(src)*3)
	for i, b := range src {
		buf := dst[i*3 : i*3+3]
		buf[0] = 0x25
		buf[1] = hex[b/16]
		buf[2] = hex[b%16]
	}
	return dst
}

func escape(s string, allowReserved bool) (escaped string) {
	if allowReserved {
		escaped = string(reserved.ReplaceAllFunc([]byte(s), pctEncode))
	} else {
		escaped = string(unreserved.ReplaceAllFunc([]byte(s), pctEncode))
	}
	return escaped
}

// A UriTemplate is a parsed representation of a URI template.
type Template struct {
	raw   string
	parts []templatePart
}

// Parse parses a URI template string into a UriTemplate object.
func Parse(raw string) (template *Template, err error) {
	template = new(Template)
	template.raw = raw
	split := strings.Split(raw, "{")
	template.parts = make([]templatePart, len(split)*2-1)
	for i, s := range split {
		if i == 0 {
			if strings.Contains(s, "}") {
				err = errors.New("unexpected }")
				break
			}
			template.parts[i].raw = s
		} else {
			subsplit := strings.Split(s, "}")
			if len(subsplit) != 2 {
				err = errors.New("malformed template")
				break
			}
			expression := subsplit[0]
			template.parts[i*2-1], err = parseExpression(expression)
			if err != nil {
				break
			}
			template.parts[i*2].raw = subsplit[1]
		}
	}
	if err != nil {
		template = nil
	}
	return template, err
}

type templatePart struct {
	raw           string
	terms         []templateTerm
	first         string
	sep           string
	named         bool
	ifemp         string
	allowReserved bool
}

type templateTerm struct {
	name     string
	explode  bool
	truncate int
}

func parseExpression(expression string) (result templatePart, err error) {
	switch expression[0] {
	case '+':
		result.sep = ","
		result.allowReserved = true
		expression = expression[1:]
	case '.':
		result.first = "."
		result.sep = "."
		expression = expression[1:]
	case '/':
		result.first = "/"
		result.sep = "/"
		expression = expression[1:]
	case ';':
		result.first = ";"
		result.sep = ";"
		result.named = true
		expression = expression[1:]
	case '?':
		result.first = "?"
		result.sep = "&"
		result.named = true
		result.ifemp = "="
		expression = expression[1:]
	case '&':
		result.first = "&"
		result.sep = "&"
		result.named = true
		result.ifemp = "="
		expression = expression[1:]
	case '#':
		result.first = "#"
		result.sep = ","
		result.allowReserved = true
		expression = expression[1:]
	default:
		result.sep = ","
	}
	rawterms := strings.Split(expression, ",")
	result.terms = make([]templateTerm, len(rawterms))
	for i, raw := range rawterms {
		result.terms[i], err = parseTerm(raw)
		if err != nil {
			break
		}
	}
	return result, err
}

func parseTerm(term string) (result templateTerm, err error) {
	if strings.HasSuffix(term, "*") {
		result.explode = true
		term = term[:len(term)-1]
	}
	split := strings.Split(term, ":")
	if len(split) == 1 {
		result.name = term
	} else if len(split) == 2 {
		result.name = split[0]
		var parsed int64
		parsed, err = strconv.ParseInt(split[1], 10, 0)
		result.truncate = int(parsed)
	} else {
		err = errors.New("multiple colons in same term")
	}
	if !validname.MatchString(result.name) {
		err = errors.New("not a valid name: " + result.name)
	}
	if result.explode && result.truncate > 0 {
		err = errors.New("both explode and prefix modifers on same term")
	}
	return result, err
}

// Expand expands a URI template with a set of values to produce a string.
func (t *Template) Expand(value interface{}) (string, error) {
	values, isMap := value.(map[string]interface{})
	if !isMap {
		if m, isMap := struct2map(value); !isMap {
			return "", errors.New("expected map[string]interface{}, struct, or pointer to struct.")
		} else {
			return t.Expand(m)
		}
	}
	var buf bytes.Buffer
	for _, p := range t.parts {
		err := p.expand(&buf, values)
		if err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

func (t *templatePart) expand(buf *bytes.Buffer, values map[string]interface{}) error {
	if len(t.raw) > 0 {
		buf.WriteString(t.raw)
		return nil
	}
	var zeroLen = buf.Len()
	buf.WriteString(t.first)
	var firstLen = buf.Len()
	for _, term := range t.terms {
		value, exists := values[term.name]
		if !exists {
			continue
		}
		if buf.Len() != firstLen {
			buf.WriteString(t.sep)
		}
		switch v := value.(type) {
		case string:
			t.expandString(buf, term, v)
		case []interface{}:
			t.expandArray(buf, term, v)
		case map[string]interface{}:
			if term.truncate > 0 {
				return errors.New("cannot truncate a map expansion")
			}
			t.expandMap(buf, term, v)
		default:
			if m, ismap := struct2map(value); ismap {
				if term.truncate > 0 {
					return errors.New("cannot truncate a map expansion")
				}
				t.expandMap(buf, term, m)
			} else {
				str := fmt.Sprintf("%v", value)
				t.expandString(buf, term, str)
			}
		}
	}
	if buf.Len() == firstLen {
		original := buf.Bytes()[:zeroLen]
		buf.Reset()
		buf.Write(original)
	}
	return nil
}

func (t *templatePart) expandName(buf *bytes.Buffer, name string, empty bool) {
	if t.named {
		buf.WriteString(name)
		if empty {
			buf.WriteString(t.ifemp)
		} else {
			buf.WriteString("=")
		}
	}
}

func (t *templatePart) expandString(buf *bytes.Buffer, term templateTerm, s string) {
	if len(s) > term.truncate && term.truncate > 0 {
		s = s[:term.truncate]
	}
	t.expandName(buf, term.name, len(s) == 0)
	buf.WriteString(escape(s, t.allowReserved))
}

func (t *templatePart) expandArray(buf *bytes.Buffer, term templateTerm, a []interface{}) {
	if len(a) == 0 {
		return
	} else if !term.explode {
		t.expandName(buf, term.name, false)
	}
	for i, value := range a {
		if term.explode && i > 0 {
			buf.WriteString(t.sep)
		} else if i > 0 {
			buf.WriteString(",")
		}
		var s string
		switch v := value.(type) {
		case string:
			s = v
		default:
			s = fmt.Sprintf("%v", v)
		}
		if len(s) > term.truncate && term.truncate > 0 {
			s = s[:term.truncate]
		}
		if t.named && term.explode {
			t.expandName(buf, term.name, len(s) == 0)
		}
		buf.WriteString(escape(s, t.allowReserved))
	}
}

func (t *templatePart) expandMap(buf *bytes.Buffer, term templateTerm, m map[string]interface{}) {
	if len(m) == 0 {
		return
	}
	if !term.explode {
		t.expandName(buf, term.name, len(m) == 0)
	}
	var firstLen = buf.Len()
	for k, value := range m {
		if firstLen != buf.Len() {
			if term.explode {
				buf.WriteString(t.sep)
			} else {
				buf.WriteString(",")
			}
		}
		var s string
		switch v := value.(type) {
		case string:
			s = v
		default:
			s = fmt.Sprintf("%v", v)
		}
		if term.explode {
			buf.WriteString(escape(k, t.allowReserved))
			buf.WriteRune('=')
			buf.WriteString(escape(s, t.allowReserved))
		} else {
			buf.WriteString(escape(k, t.allowReserved))
			buf.WriteRune(',')
			buf.WriteString(escape(s, t.allowReserved))
		}
	}
}

func struct2map(v interface{}) (map[string]interface{}, bool) {
	value := reflect.ValueOf(v)
	switch value.Type().Kind() {
	case reflect.Ptr:
		return struct2map(value.Elem().Interface())
	case reflect.Struct:
		m := make(map[string]interface{})
		for i := 0; i < value.NumField(); i++ {
			tag := value.Type().Field(i).Tag
			var name string
			if strings.Contains(string(tag), ":") {
				name = tag.Get("uri")
			} else {
				name = strings.TrimSpace(string(tag))
			}
			if len(name) == 0 {
				name = value.Type().Field(i).Name
			}
			m[name] = value.Field(i).Interface()
		}
		return m, true
	}
	return nil, false
}
