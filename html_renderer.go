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
	"bytes"
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
	// If FilterTag is nil, then no filtering will occur.
	//
	// FilterTag functions must not modify the byte slice
	// nor retain the slice after the function returns.
	FilterTag func(tag []byte) bool
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
	dst      []byte
	lowerBuf []byte
}

func (r *renderState) openTagAttr(name atom.Atom) {
	start := len(r.dst)
	r.dst = append(r.dst, '<')
	r.dst = append(r.dst, name.String()...)
	if r.FilterTag != nil && r.FilterTag(r.dst[start+1:]) {
		r.dst = r.dst[:start]
		r.dst = append(r.dst, "&lt;"...)
		r.dst = append(r.dst, name.String()...)
	}
}

func (r *renderState) openTag(name atom.Atom) {
	r.openTagAttr(name)
	r.dst = append(r.dst, '>')
}

func (r *renderState) closeTag(name atom.Atom) {
	const prefix = "</"
	start := len(r.dst)
	r.dst = append(r.dst, "</"...)
	r.dst = append(r.dst, name.String()...)
	if r.FilterTag != nil && r.FilterTag(r.dst[start+1:]) {
		r.dst = r.dst[:start]
		r.dst = append(r.dst, "&lt;/"...)
		r.dst = append(r.dst, name.String()...)
	}
	r.dst = append(r.dst, '>')
}

func (r *renderState) block(source []byte, block *Block) {
	switch block.Kind() {
	case ParagraphKind:
		r.openTag(atom.P)
		r.children(source, block, false)
		r.closeTag(atom.P)
	case ThematicBreakKind:
		r.openTag(atom.Hr)
	case ATXHeadingKind, SetextHeadingKind:
		var tagName atom.Atom
		switch block.HeadingLevel() {
		case 1:
			tagName = atom.H1
		case 2:
			tagName = atom.H2
		case 3:
			tagName = atom.H3
		case 4:
			tagName = atom.H4
		case 5:
			tagName = atom.H5
		default:
			tagName = atom.H6
		}
		r.openTag(tagName)
		r.children(source, block, false)
		r.closeTag(tagName)
	case IndentedCodeBlockKind, FencedCodeBlockKind:
		r.openTag(atom.Pre)
		r.openTagAttr(atom.Code)
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
		r.closeTag(atom.Code)
		r.closeTag(atom.Pre)
	case BlockQuoteKind:
		r.openTag(atom.Blockquote)
		r.children(source, block, false)
		r.closeTag(atom.Blockquote)
	case ListKind:
		var tagName atom.Atom
		if block.IsOrderedList() {
			tagName = atom.Ol
			r.openTagAttr(tagName)
			if n := block.firstChild().Block().ListItemNumber(source); n >= 0 && n != 1 {
				r.dst = append(r.dst, ` start="`...)
				r.dst = strconv.AppendInt(r.dst, int64(n), 10)
				r.dst = append(r.dst, `"`...)
			}
			r.dst = append(r.dst, ">"...)
		} else {
			tagName = atom.Ul
			r.openTag(tagName)
		}
		r.children(source, block, false)
		r.closeTag(tagName)
	case ListItemKind:
		r.openTag(atom.Li)
		r.children(source, block, block.IsTightList())
		r.closeTag(atom.Li)
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
			if r.FilterTag == nil {
				r.dst = append(r.dst, spanSlice(source, inline.Span())...)
			} else {
				r.filterRaw(spanSlice(source, inline.Span()))
			}
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
		r.openTag(atom.Em)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.closeTag(atom.Em)
	case StrongKind:
		r.openTag(atom.Strong)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.closeTag(atom.Strong)
	case CodeSpanKind:
		r.openTag(atom.Code)
		for _, c := range inline.children {
			r.inline(source, c)
		}
		r.closeTag(atom.Code)
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
		r.openTagAttr(atom.A)
		r.dst = append(r.dst, ` href="`...)
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
		r.closeTag(atom.A)
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
		r.openTagAttr(atom.Img)
		r.dst = append(r.dst, ` src="`...)
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
		r.openTagAttr(atom.A)
		r.dst = append(r.dst, ` href="`...)
		if IsEmailAddress(destination) {
			r.dst = append(r.dst, "mailto:"...)
		}
		r.dst = append(r.dst, html.EscapeString(NormalizeURI(destination))...)
		r.dst = append(r.dst, `">`...)
		r.dst = append(r.dst, html.EscapeString(destination)...)
		r.closeTag(atom.A)
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

// filterRaw performs the tag filtering
// described in https://github.github.com/gfm/#disallowed-raw-html-extension-.
//
// It cannot use a conventional HTML parser,
// since raw HTML in Markdown may be incomplete or start in the middle of a tag.
func (r *renderState) filterRaw(rawHTML []byte) {
	const (
		copyState = iota
		commentState
		piState
		declState
		cdataState
	)
	state := copyState
	copyStart := 0
	for i := 0; i < len(rawHTML); {
		switch state {
		case copyState:
			if rawHTML[i] == '<' {
				switch {
				case hasBytePrefix(rawHTML[i:], cdataPrefix):
					state = cdataState
					i += len(cdataPrefix)
				case hasBytePrefix(rawHTML[i:], htmlCommentPrefix):
					state = commentState
					i += len(htmlCommentPrefix)
				case hasHTMLDeclarationPrefix(rawHTML[i:]):
					state = declState
					i += len("<!x")
				default:
					tagNameStart := i + 1
					tagEnd := len(rawHTML)
					if j := bytes.IndexByte(rawHTML[tagNameStart:], '>'); j >= 0 {
						tagEnd = tagNameStart + j + len(">")
					}
					tagNameEnd := tagNameStart + htmlTagNameEnd(rawHTML[tagNameStart:tagEnd])
					tagName := maybeLower(rawHTML[tagNameStart:tagNameEnd], &r.lowerBuf)
					if r.FilterTag(tagName) {
						r.dst = append(r.dst, rawHTML[copyStart:i]...)
						r.dst = append(r.dst, "&lt;"...)
						r.dst = append(r.dst, rawHTML[tagNameStart:tagEnd]...)
						copyStart = tagEnd
					}
					i = tagEnd
				}
			} else {
				i++
			}
		case commentState:
			if hasBytePrefix(rawHTML[i:], htmlCommentSuffix) {
				state = copyState
				i += len(htmlCommentSuffix)
			} else {
				i++
			}
		case piState:
			if hasBytePrefix(rawHTML[i:], processingInstructionSuffix) {
				state = copyState
				i += len(processingInstructionSuffix)
			} else {
				i++
			}
		case declState:
			if rawHTML[i] == '>' {
				state = copyState
			}
			i++
		case cdataState:
			if hasBytePrefix(rawHTML[i:], cdataSuffix) {
				state = copyState
				i += len(cdataSuffix)
			} else {
				i++
			}
		default:
			panic("unreachable")
		}
	}

	r.dst = append(r.dst, rawHTML[copyStart:]...)
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

// FilterTagGFM performs the same tag filtering as the
// GitHub Flavored Markdown [tagfilter extension].
// It is suitable for use as the FilterTag field in [HTMLRenderer].
//
// [tagfilter extension]: https://github.github.com/gfm/#disallowed-raw-html-extension-
func FilterTagGFM(tag []byte) bool {
	tagAtom := atom.Lookup(tag)
	return tagAtom == atom.Title ||
		tagAtom == atom.Textarea ||
		tagAtom == atom.Style ||
		tagAtom == atom.Xmp ||
		tagAtom == atom.Iframe ||
		tagAtom == atom.Noembed ||
		tagAtom == atom.Noframes ||
		tagAtom == atom.Script ||
		tagAtom == atom.Plaintext
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
