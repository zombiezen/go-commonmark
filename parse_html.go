// Copyright 2023 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//		 https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package commonmark

import (
	"strings"

	"golang.org/x/net/html/atom"
)

func parseHTMLTag(r *inlineByteReader) Span {
	const (
		cdataPrefix = "[CDATA["
		cdataSuffix = "]]>"
	)

	if r.current() != '<' {
		return NullSpan()
	}
	result := Span{Start: r.pos, End: -1}
	if !r.next() || r.jumped() {
		return NullSpan()
	}
	switch r.current() {
	case '?':
		// Processing instructions.
		if !r.next() {
			return NullSpan()
		}
		for {
			if r.current() != '?' {
				if !r.next() {
					return NullSpan()
				}
				continue
			}
			if !r.next() || r.jumped() {
				return NullSpan()
			}
			if r.current() == '>' {
				result.End = r.pos + 1
				r.next()
				return result
			}
		}
	case '!':
		// Declaration, comment, or CDATA.
		if !r.next() || r.jumped() {
			return NullSpan()
		}
		rest := r.remainingNodeBytes()
		switch {
		case len(rest) > 0 && isASCIILetter(rest[0]):
			// Declaration.
			r.next()
			for r.current() != '>' {
				if !r.next() {
					return NullSpan()
				}
			}
			result.End = r.pos + 1
			r.next()
			return result
		case hasBytePrefix(rest, "--"):
			// Comment.
			r.next()
			if !r.next() || r.jumped() {
				return NullSpan()
			}
			if textStart := r.remainingNodeBytes(); hasBytePrefix(textStart, ">") || hasBytePrefix(textStart, "->") {
				return NullSpan()
			}
			for {
				if hasBytePrefix(r.remainingNodeBytes(), "-->") {
					r.next()
					r.next()
					result.End = r.pos + 1
					r.next()
					return result
				}
				// Check for either "--" or "--->".
				if hasBytePrefix(r.remainingNodeBytes(), "--") {
					return NullSpan()
				}
				if !r.next() {
					return NullSpan()
				}
			}
		case hasBytePrefix(rest, cdataPrefix):
			// CDATA.
			for i := 0; i < len(cdataPrefix); i++ {
				if !r.next() {
					return NullSpan()
				}
			}
			for {
				if hasBytePrefix(r.remainingNodeBytes(), cdataSuffix) {
					for i := 0; i < len(cdataSuffix)-1; i++ {
						r.next()
					}
					result.End = r.pos + 1
					r.next()
					return result
				}
				if !r.next() {
					return NullSpan()
				}
			}
		default:
			return NullSpan()
		}
	case '/':
		result.End = parseHTMLClosingTag(r)
		if result.End < 0 {
			return NullSpan()
		}
		return result
	default:
		result.End = parseHTMLOpenTag(r)
		if result.End < 0 {
			return NullSpan()
		}
		return result
	}
}

// parseHTMLOpenTag parses an [open tag] sans the leading '<'.
//
// [open tag]: https://spec.commonmark.org/0.30/#open-tag
func parseHTMLOpenTag(r *inlineByteReader) (end int) {
	if !parseHTMLTagName(r) {
		return -1
	}
	for {
		beforeSpace := r.pos
		if !skipLinkSpace(r) {
			return -1
		}
		switch r.current() {
		case '/':
			if !r.next() || r.jumped() {
				return -1
			}
			if r.current() != '>' {
				return -1
			}
			fallthrough
		case '>':
			end = r.pos + 1
			r.next()
			return end
		}
		if r.pos == beforeSpace || !parseHTMLAttribute(r) {
			return -1
		}
	}
}

// parseHTMLClosingTag parses an [open tag] sans the leading '<'.
//
// [closing tag]: https://spec.commonmark.org/0.30/#closing-tag
func parseHTMLClosingTag(r *inlineByteReader) (end int) {
	if r.current() != '/' {
		return -1
	}
	if !r.next() || r.jumped() {
		return -1
	}
	if !parseHTMLTagName(r) {
		return -1
	}
	if !skipLinkSpace(r) {
		return -1
	}
	if r.current() != '>' {
		return -1
	}
	end = r.pos + 1
	r.next()
	return end
}

func parseHTMLTagName(r *inlineByteReader) bool {
	if !isASCIILetter(r.current()) {
		return false
	}
	if !r.next() {
		return true
	}
	for isASCIILetter(r.current()) || isASCIIDigit(r.current()) || r.current() == '-' {
		if !r.next() {
			return true
		}
	}
	return true
}

func parseHTMLAttribute(r *inlineByteReader) bool {
	// Attribute name.
	if c := r.current(); !isASCIILetter(c) && c != '_' && c != ':' {
		return false
	}
	if !r.next() {
		// Only one character needed for name and value is optional.
		return true
	}
	for isASCIILetter(r.current()) || isASCIIDigit(r.current()) || strings.IndexByte("_.:-", r.current()) >= 0 {
		if !r.next() {
			return true
		}
	}

	// Attribute value specification.
	// Don't consume space unless it is followed by an equal sign,
	// since it will cause future attributes to fail.
	prevState := *r
	if !skipLinkSpace(r) {
		*r = prevState
		return true
	}
	if r.current() != '=' {
		*r = prevState
		return true
	}
	if !r.next() {
		// Must have an attribute value following equals sign.
		return false
	}
	if !skipLinkSpace(r) {
		// Must have an attribute value following equals sign.
		return false
	}
	switch c := r.current(); {
	case c == '\'':
		if !r.next() {
			return false
		}
		for r.current() != '\'' {
			if !r.next() {
				return false
			}
		}
		r.next()
		return true
	case c == '"':
		if !r.next() {
			return false
		}
		for r.current() != '"' {
			if !r.next() {
				return false
			}
		}
		r.next()
		return true
	case isUnquotedAttributeValueChar(c):
		for r.next() && isUnquotedAttributeValueChar(r.current()) {
		}
		return true
	default:
		return false
	}
}

// htmlBlockConditions is the set of [HTML block] start and end conditions.
//
// [HTML block]: https://spec.commonmark.org/0.30/#html-blocks
var htmlBlockConditions = []struct {
	startCondition        func(line []byte) bool
	endCondition          func(line []byte) bool
	canInterruptParagraph bool
}{
	{
		startCondition: func(line []byte) bool {
			for _, starter := range htmlBlockStarters1 {
				if hasCaseInsensitiveBytePrefix(line, starter) {
					rest := line[len(starter):]
					if len(rest) == 0 || isSpaceTabOrLineEnding(rest[0]) || rest[0] == '>' {
						return true
					}
				}
			}
			return false
		},
		endCondition: func(line []byte) bool {
			for _, ender := range htmlBlockEnders1 {
				if caseInsensitiveContains(line, ender) {
					return true
				}
			}
			return false
		},
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			return hasBytePrefix(line, "<!--")
		},
		endCondition: func(line []byte) bool {
			return contains(line, "-->")
		},
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			return hasBytePrefix(line, "<?")
		},
		endCondition: func(line []byte) bool {
			return contains(line, "?>")
		},
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			return hasBytePrefix(line, "<!") && len(line) >= 3 && isASCIILetter(line[2])
		},
		endCondition: func(line []byte) bool {
			return contains(line, ">")
		},
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			return hasBytePrefix(line, "<![CDATA[")
		},
		endCondition: func(line []byte) bool {
			return contains(line, "]]>")
		},
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			switch {
			case hasBytePrefix(line, "</"):
				line = line[2:]
			case hasBytePrefix(line, "<"):
				line = line[1:]
			default:
				return false
			}
			for _, starter := range htmlBlockStarters6 {
				if hasCaseInsensitiveBytePrefix(line, starter) {
					rest := line[len(starter):]
					if len(rest) == 0 || isSpaceTabOrLineEnding(rest[0]) || rest[0] == '>' || hasBytePrefix(rest, "/>") {
						return true
					}
				}
			}
			return false
		},
		endCondition:          isBlankLine,
		canInterruptParagraph: true,
	},
	{
		startCondition: func(line []byte) bool {
			if !hasBytePrefix(line, "<") {
				return false
			}
			fakeInline := &Inline{
				kind: UnparsedKind,
				span: Span{Start: 1, End: len(line)},
			}
			nodes := []*Inline{fakeInline}
			r := newInlineByteReader(line, nodes, 1)
			if hasBytePrefix(line, "</") {
				if parseHTMLClosingTag(r) < 0 {
					return false
				}
			} else {
				if parseHTMLOpenTag(r) < 0 {
					return false
				}
			}
			return !skipLinkSpace(r)
		},
		endCondition:          isBlankLine,
		canInterruptParagraph: false,
	},
}

func hasCaseInsensitiveBytePrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i, bb := range b[:len(prefix)] {
		if toLowerASCII(prefix[i]) != toLowerASCII(bb) {
			return false
		}
	}
	return true
}

func caseInsensitiveContains(b []byte, search string) bool {
	for i := 0; i < len(b)-len(search); i++ {
		if hasCaseInsensitiveBytePrefix(b[i:], search) {
			return true
		}
	}
	return false
}

func toLowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

func isUnquotedAttributeValueChar(c byte) bool {
	return !isSpaceTabOrLineEnding(c) && strings.IndexByte("\"'=<>`", c) < 0
}

var (
	htmlBlockStarters1 = []string{
		"<pre",
		"<script",
		"<style",
		"<textarea",
	}
	htmlBlockEnders1 = []string{
		"</pre>",
		"</script>",
		"</style>",
		"</textarea>",
	}

	htmlBlockStarters6 = []string{
		atom.Address.String(),
		atom.Article.String(),
		atom.Aside.String(),
		atom.Base.String(),
		atom.Basefont.String(),
		atom.Blockquote.String(),
		atom.Body.String(),
		atom.Caption.String(),
		atom.Center.String(),
		atom.Col.String(),
		atom.Colgroup.String(),
		atom.Dd.String(),
		atom.Details.String(),
		atom.Dialog.String(),
		atom.Dir.String(),
		atom.Div.String(),
		atom.Dl.String(),
		atom.Dt.String(),
		atom.Fieldset.String(),
		atom.Figcaption.String(),
		atom.Figure.String(),
		atom.Footer.String(),
		atom.Form.String(),
		atom.Frame.String(),
		atom.Frameset.String(),
		atom.H1.String(),
		atom.H2.String(),
		atom.H3.String(),
		atom.H4.String(),
		atom.H5.String(),
		atom.H6.String(),
		atom.Head.String(),
		atom.Header.String(),
		atom.Hr.String(),
		atom.Html.String(),
		atom.Iframe.String(),
		atom.Legend.String(),
		atom.Li.String(),
		atom.Link.String(),
		atom.Main.String(),
		atom.Menu.String(),
		atom.Menuitem.String(),
		atom.Nav.String(),
		atom.Noframes.String(),
		atom.Ol.String(),
		atom.Optgroup.String(),
		atom.Option.String(),
		atom.P.String(),
		atom.Param.String(),
		atom.Section.String(),
		atom.Source.String(),
		atom.Summary.String(),
		atom.Table.String(),
		atom.Tbody.String(),
		atom.Td.String(),
		atom.Tfoot.String(),
		atom.Th.String(),
		atom.Thead.String(),
		atom.Title.String(),
		atom.Tr.String(),
		atom.Track.String(),
		atom.Ul.String(),
	}
)
