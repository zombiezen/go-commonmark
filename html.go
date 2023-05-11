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

//go:generate stringer -type=SoftBreakBehavior -output=html_string.go

package commonmark

import (
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/atom"
)

// An HTMLRenderer converts fully parsed CommonMark blocks into HTML.
//
// # Security considerations
//
// CommonMark permits the use of [raw HTML], which can introduce
// [Cross-Site Scripting (XSS)] vulnerabilities and [HTML parse errors]
// when used with untrusted inputs.
// There are a few options to mitigate this risk:
//
//   - The resulting HTML can be sent through an HTML sanitizer.
//     This is highly recommended.
//   - Set IgnoreRaw to prevent inclusion of raw HTML.
//     This eliminates any raw HTML usage,
//     so the output is guaranteed to use a fixed set of elements
//     and avoid parse errors.
//     However, this can lead to content being omitted from the document entirely,
//     which may be surprising to end-users for legitimate use cases.
//   - FilterTag can be used to prevent some tags from being used
//     while still showing the source text.
//     The default FilterTag prevents common undesirable tags from being used.
//     Note that this does not prevent parse errors.
//     For untrusted inputs, this technique should be combined with sanitization.
//
// [Cross-Site Scripting (XSS)]: https://owasp.org/www-community/attacks/xss/
// [HTML parse errors]: https://html.spec.whatwg.org/multipage/parsing.html#parse-errors
// [raw HTML]: https://spec.commonmark.org/0.30/#raw-html
type HTMLRenderer struct {
	// ReferenceMap holds the document's link reference definitions.
	ReferenceMap ReferenceMap
	// SoftBreakBehavior determines how soft line breaks are rendered.
	SoftBreakBehavior SoftBreakBehavior
	// If IgnoreRaw is true, the renderer skips any HTML blocks or raw HTML.
	IgnoreRaw bool
	// FilterTag is a predicate function
	// that reports whether an element with the given lowercased tag name
	// should have its leading angle bracket escaped.
	// If FilterTag is nil, then a filter
	// equivalent to the GitHub Flavored Markdown [tagfilter extension]
	// will be used.
	// It has no effect if IgnoreRaw is true.
	//
	// FilterTag functions must not modify the byte slice
	// nor retain the slice after the function returns.
	//
	// [tagfilter extension]: https://github.github.com/gfm/#disallowed-raw-html-extension-
	FilterTag func(tag []byte) bool
	// If SkipFilter is true, FilterTag will not be consulted
	// and any raw HTML is passed through verbatim.
	// This avoids the performance penalty of tokenizing the raw HTML.
	// It has no effect if IgnoreRaw is true.
	SkipFilter bool
}

// RenderHTML writes the given sequence of parsed blocks
// to the given writer as HTML
// using the default options for [HTMLRenderer].
// It will return the first error encountered, if any.
func RenderHTML(w io.Writer, blocks []*RootBlock, refMap ReferenceMap) error {
	return (&HTMLRenderer{ReferenceMap: refMap}).Render(w, blocks)
}

// Render writes the given sequence of parsed blocks
// to the given writer as HTML.
// It will return the first error encountered, if any.
func (r *HTMLRenderer) Render(w io.Writer, blocks []*RootBlock) error {
	var buf []byte
	for i, b := range blocks {
		buf = buf[:0]
		if i > 0 {
			buf = append(buf, "\n\n"...)
		}
		buf = r.AppendBlock(buf, b)
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("render markdown to html: %w", err)
		}
	}
	return nil
}

// AppendBlock appends the rendered HTML of a fully parsed block to dst
// and returns the resulting byte slice.
func (r *HTMLRenderer) AppendBlock(dst []byte, block *RootBlock) []byte {
	state := &renderState{
		HTMLRenderer: r,
		dst:          dst,
	}
	state.block(block.Source, &block.Block)
	return state.dst
}

type renderState struct {
	*HTMLRenderer
	dst []byte
}

func (r *renderState) block(source []byte, block *Block) {
	switch block.Kind() {
	case ParagraphKind:
		r.dst = append(r.dst, "<p>"...)
		r.children(source, block, false)
		r.dst = append(r.dst, "</p>"...)
	case ThematicBreakKind:
		r.dst = append(r.dst, "<hr>"...)
	case ATXHeadingKind, SetextHeadingKind:
		level := block.HeadingLevel()
		r.dst = append(r.dst, "<h"...)
		r.dst = strconv.AppendInt(r.dst, int64(level), 10)
		r.dst = append(r.dst, ">"...)
		r.children(source, block, false)
		r.dst = append(r.dst, "</h"...)
		r.dst = strconv.AppendInt(r.dst, int64(level), 10)
		r.dst = append(r.dst, ">"...)
	case IndentedCodeBlockKind, FencedCodeBlockKind:
		r.dst = append(r.dst, "<pre><code"...)
		if info := block.InfoString(); info != nil {
			words := strings.Fields(info.Text(source))
			if len(words) > 0 {
				r.dst = append(r.dst, ` class="language-`...)
				r.dst = append(r.dst, html.EscapeString(words[0])...)
				r.dst = append(r.dst, `"`...)
			}
		}
		r.dst = append(r.dst, ">"...)
		r.children(source, block, false)
		r.dst = append(r.dst, "</code></pre>"...)
	case BlockQuoteKind:
		r.dst = append(r.dst, "<blockquote>"...)
		r.children(source, block, false)
		r.dst = append(r.dst, "</blockquote>"...)
	case ListKind:
		if block.IsOrderedList() {
			r.dst = append(r.dst, "<ol"...)
			if n := block.firstChild().Block().ListItemNumber(source); n >= 0 && n != 1 {
				r.dst = append(r.dst, ` start="`...)
				r.dst = strconv.AppendInt(r.dst, int64(n), 10)
				r.dst = append(r.dst, `"`...)
			}
			r.dst = append(r.dst, ">"...)
		} else {
			r.dst = append(r.dst, "<ul>"...)
		}
		r.children(source, block, false)
		if block.IsOrderedList() {
			r.dst = append(r.dst, "</ol>"...)
		} else {
			r.dst = append(r.dst, "</ul>"...)
		}
	case ListItemKind:
		r.dst = append(r.dst, "<li>"...)
		r.children(source, block, block.IsTightList())
		r.dst = append(r.dst, "</li>"...)
	case HTMLBlockKind:
		if !r.IgnoreRaw {
			r.children(source, block, false)
		}
	}
}

func (r *renderState) children(source []byte, parent *Block, tight bool) {
	switch {
	case parent != nil && len(parent.inlineChildren) > 0:
		for _, c := range parent.inlineChildren {
			r.inline(source, c)
		}
	case parent != nil && len(parent.blockChildren) > 0:
		for _, c := range parent.blockChildren {
			if tight && c.Kind() == ParagraphKind {
				r.children(source, c, false)
			} else {
				r.block(source, c)
			}
		}
	}
}

func (r *renderState) inline(source []byte, inline *Inline) {
	const hardLineBreak = "<br>\n"
	switch inline.Kind() {
	case TextKind, UnparsedKind:
		r.dst = escapeHTML(r.dst, spanSlice(source, inline.Span()))
	case CharacterReferenceKind:
		r.dst = append(r.dst, spanSlice(source, inline.Span())...)
	case RawHTMLKind:
		if !r.IgnoreRaw {
			// TODO(now): Filter if !r.SkipFilter
			r.dst = append(r.dst, spanSlice(source, inline.Span())...)
		}
	case SoftLineBreakKind:
		switch r.SoftBreakBehavior {
		case SoftBreakHarden:
			r.dst = append(r.dst, hardLineBreak...)
		case SoftBreakSpace:
			r.dst = append(r.dst, ' ')
		default:
			if inline.Span().Len() > 0 {
				r.dst = append(r.dst, spanSlice(source, inline.Span())...)
			} else {
				r.dst = append(r.dst, '\n')
			}
		}
	case HardLineBreakKind:
		r.dst = append(r.dst, hardLineBreak...)
	case EmphasisKind:
		r.dst = append(r.dst, "<em>"...)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.dst = append(r.dst, "</em>"...)
	case StrongKind:
		r.dst = append(r.dst, "<strong>"...)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.dst = append(r.dst, "</strong>"...)
	case CodeSpanKind:
		r.dst = append(r.dst, "<code>"...)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.dst = append(r.dst, "</code>"...)
	case LinkKind:
		var def LinkDefinition
		if ref := inline.LinkReference(); ref != "" {
			def = r.ReferenceMap[ref]
		} else {
			title := inline.LinkTitle()
			def = LinkDefinition{
				Destination:  inline.LinkDestination().Text(source),
				Title:        title.Text(source),
				TitlePresent: title != nil,
			}
		}
		r.dst = append(r.dst, `<a href="`...)
		r.dst = append(r.dst, html.EscapeString(NormalizeURI(def.Destination))...)
		r.dst = append(r.dst, `"`...)
		if def.TitlePresent {
			r.dst = append(r.dst, ` title="`...)
			r.dst = append(r.dst, html.EscapeString(def.Title)...)
			r.dst = append(r.dst, `"`...)
		}
		r.dst = append(r.dst, ">"...)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.dst = append(r.dst, "</a>"...)
	case ImageKind:
		var def LinkDefinition
		if ref := inline.LinkReference(); ref != "" {
			def = r.ReferenceMap[ref]
		} else {
			title := inline.LinkTitle()
			def = LinkDefinition{
				Destination:  inline.LinkDestination().Text(source),
				Title:        title.Text(source),
				TitlePresent: title != nil,
			}
		}
		r.dst = append(r.dst, `<img src="`...)
		r.dst = append(r.dst, html.EscapeString(NormalizeURI(def.Destination))...)
		r.dst = append(r.dst, `"`...)
		if def.TitlePresent {
			r.dst = append(r.dst, ` title="`...)
			r.dst = append(r.dst, html.EscapeString(def.Title)...)
			r.dst = append(r.dst, `"`...)
		}
		r.dst = appendAltText(r.dst, source, inline)
		r.dst = append(r.dst, ">"...)
	case AutolinkKind:
		destination := inline.children[0].Text(source)
		r.dst = append(r.dst, `<a href="`...)
		if IsEmailAddress(destination) {
			r.dst = append(r.dst, "mailto:"...)
		}
		r.dst = append(r.dst, html.EscapeString(NormalizeURI(destination))...)
		r.dst = append(r.dst, `">`...)
		r.dst = append(r.dst, html.EscapeString(destination)...)
		r.dst = append(r.dst, "</a>"...)
	case IndentKind:
		for i, n := 0, inline.IndentWidth(); i < n; i++ {
			r.dst = append(r.dst, ' ')
		}
	case HTMLTagKind:
		for _, c := range inline.children {
			r.inline(source, c)
		}
	}
}

func appendAltText(dst []byte, source []byte, parent *Inline) []byte {
	stack := []*Inline{parent}
	hasAttr := false
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch curr.Kind() {
		case TextKind:
			if !hasAttr {
				dst = append(dst, ` alt="`...)
				hasAttr = true
			}
			dst = append(dst, curr.Text(source)...)
		case IndentKind, SoftLineBreakKind, HardLineBreakKind:
			if !hasAttr {
				dst = append(dst, ` alt="`...)
				hasAttr = true
			}
			dst = append(dst, ' ')
		case LinkDestinationKind, LinkTitleKind, LinkLabelKind:
			// Ignore.
		default:
			for i := len(curr.children) - 1; i >= 0; i-- {
				stack = append(stack, curr.children[i])
			}
		}
	}
	if !hasAttr {
		dst = append(dst, `alt="`...)
	}
	dst = append(dst, `"`...)
	return dst
}

// escapeHTML appends the HTML-escaped version of a byte slice to another byte slice.
func escapeHTML(dst []byte, src []byte) []byte {
	verbatimStart := 0
	for i, b := range src {
		switch b {
		case '&':
			dst = append(dst, src[verbatimStart:i]...)
			dst = append(dst, "&amp;"...)
			verbatimStart = i + 1
		case '\'':
			dst = append(dst, src[verbatimStart:i]...)
			// "&#39;" is shorter than "&apos;" and apos was not in HTML until HTML5.
			dst = append(dst, "&#39;"...)
			verbatimStart = i + 1
		case '<':
			dst = append(dst, src[verbatimStart:i]...)
			dst = append(dst, "&lt;"...)
			verbatimStart = i + 1
		case '>':
			dst = append(dst, src[verbatimStart:i]...)
			dst = append(dst, "&gt;"...)
			verbatimStart = i + 1
		case '"':
			dst = append(dst, src[verbatimStart:i]...)
			dst = append(dst, "&quot;"...)
			verbatimStart = i + 1
		}
	}
	if verbatimStart < len(src) {
		dst = append(dst, src[verbatimStart:]...)
	}
	return dst
}

func maybeLower(x []byte, buf *[]byte) []byte {
	hasUpper := false
	for _, b := range x {
		if 'A' <= b && b <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return x
	}

	*buf = (*buf)[:0]
	for _, b := range x {
		if 'A' <= b && b <= 'Z' {
			*buf = append(*buf, b-'A'+'a')
		} else {
			*buf = append(*buf, b)
		}
	}
	return *buf
}

// SoftBreakBehavior is an enumeration of rendering styles for [soft line breaks].
//
// [soft line breaks]: https://spec.commonmark.org/0.30/#soft-line-breaks
type SoftBreakBehavior int

const (
	// SoftBreakPreserve indicates that a soft line break should be rendered as-is.
	SoftBreakPreserve SoftBreakBehavior = iota
	// SoftBreakSpace indicates that a soft line break should be rendered as a space.
	SoftBreakSpace
	// SoftBreakHarden indicates that a soft line break should be rendered as a hard line break.
	SoftBreakHarden
)

// NormalizeURI percent-encodes any characters in a string
// that are not reserved or unreserved URI characters.
// This is commonly used for transforming CommonMark link destinations
// into strings suitable for href or src attributes.
func NormalizeURI(s string) string {
	// RFC 3986 reserved and unreserved characters.
	const safeSet = `;/?:@&=+$,-_.!~*'()#`

	sb := new(strings.Builder)
	sb.Grow(len(s))
	skip := 0
	var buf [utf8.UTFMax]byte
	for i, c := range s {
		if skip > 0 {
			skip--
			sb.WriteRune(c)
			continue
		}
		switch {
		case c == '%':
			if i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
				skip = 2
				sb.WriteByte('%')
			} else {
				sb.WriteString("%25")
			}
		case (c < 0x80 && (isASCIILetter(byte(c)) || isASCIIDigit(byte(c)))) || strings.ContainsRune(safeSet, c):
			sb.WriteRune(c)
		default:
			n := utf8.EncodeRune(buf[:], c)
			for _, b := range buf[:n] {
				sb.WriteByte('%')
				sb.WriteByte(urlHexDigit(b >> 4))
				sb.WriteByte(urlHexDigit(b & 0x0f))
			}
		}
	}
	return sb.String()
}

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

func isHex(c byte) bool {
	return 'a' <= c && c <= 'f' || 'A' <= c && c <= 'f' || isASCIIDigit(c)
}

func urlHexDigit(x byte) byte {
	switch {
	case x < 0xa:
		return '0' + x
	case x < 0x10:
		return 'A' + x - 0xa
	default:
		panic("out of bounds")
	}
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
