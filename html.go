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
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

func RenderHTML(w io.Writer, blocks []*RootBlock, refMap ReferenceMap) error {
	var buf []byte
	for i, b := range blocks {
		buf = buf[:0]
		if i > 0 {
			buf = append(buf, "\n\n"...)
		}
		buf = appendHTML(buf, b.Source, refMap, &b.Block)
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("render markdown to html: %w", err)
		}
	}
	return nil
}

func appendHTML(dst []byte, source []byte, refMap ReferenceMap, block *Block) []byte {
	switch block.Kind() {
	case ParagraphKind:
		dst = append(dst, "<p>"...)
		dst = appendChildrenHTML(dst, source, refMap, block, false)
		dst = append(dst, "</p>"...)
	case ThematicBreakKind:
		dst = append(dst, "<hr>"...)
	case ATXHeadingKind, SetextHeadingKind:
		level := block.HeadingLevel()
		dst = append(dst, "<h"...)
		dst = strconv.AppendInt(dst, int64(level), 10)
		dst = append(dst, ">"...)
		dst = appendChildrenHTML(dst, source, refMap, block, false)
		dst = append(dst, "</h"...)
		dst = strconv.AppendInt(dst, int64(level), 10)
		dst = append(dst, ">"...)
	case IndentedCodeBlockKind, FencedCodeBlockKind:
		dst = append(dst, "<pre><code"...)
		if info := block.InfoString(); info != nil {
			words := strings.Fields(info.Text(source))
			if len(words) > 0 {
				dst = append(dst, ` class="language-`...)
				dst = append(dst, html.EscapeString(words[0])...)
				dst = append(dst, `"`...)
			}
		}
		dst = append(dst, ">"...)
		dst = appendChildrenHTML(dst, source, refMap, block, false)
		dst = append(dst, "</code></pre>"...)
	case BlockQuoteKind:
		dst = append(dst, "<blockquote>"...)
		dst = appendChildrenHTML(dst, source, refMap, block, false)
		dst = append(dst, "</blockquote>"...)
	case ListKind:
		if block.IsOrderedList() {
			dst = append(dst, "<ol"...)
			if n := block.firstChild().Block().ListItemNumber(source); n >= 0 && n != 1 {
				dst = append(dst, ` start="`...)
				dst = strconv.AppendInt(dst, int64(n), 10)
				dst = append(dst, `"`...)
			}
			dst = append(dst, ">"...)
		} else {
			dst = append(dst, "<ul>"...)
		}
		dst = appendChildrenHTML(dst, source, refMap, block, false)
		if block.IsOrderedList() {
			dst = append(dst, "</ol>"...)
		} else {
			dst = append(dst, "</ul>"...)
		}
	case ListItemKind:
		dst = append(dst, "<li>"...)
		dst = appendChildrenHTML(dst, source, refMap, block, block.IsTightList())
		dst = append(dst, "</li>"...)
	}
	return dst
}

func appendChildrenHTML(dst []byte, source []byte, refMap ReferenceMap, parent *Block, tight bool) []byte {
	switch {
	case parent != nil && len(parent.inlineChildren) > 0:
		for _, c := range parent.inlineChildren {
			dst = appendInlineHTML(dst, source, refMap, c)
		}
	case parent != nil && len(parent.blockChildren) > 0:
		for _, c := range parent.blockChildren {
			if tight && c.Kind() == ParagraphKind {
				dst = appendChildrenHTML(dst, source, refMap, c, false)
			} else {
				dst = appendHTML(dst, source, refMap, c)
			}
		}
	}
	return dst
}

func appendInlineHTML(dst []byte, source []byte, refMap ReferenceMap, inline *Inline) []byte {
	switch inline.Kind() {
	case TextKind, UnparsedKind:
		dst = append(dst, html.EscapeString(inline.Text(source))...)
	case SoftLineBreakKind:
		dst = append(dst, '\n')
	case HardLineBreakKind:
		dst = append(dst, "<br>\n"...)
	case EmphasisKind:
		dst = append(dst, "<em>"...)
		for _, c := range inline.children {
			dst = appendInlineHTML(dst, source, refMap, c)
		}
		dst = append(dst, "</em>"...)
	case StrongKind:
		dst = append(dst, "<strong>"...)
		for _, c := range inline.children {
			dst = appendInlineHTML(dst, source, refMap, c)
		}
		dst = append(dst, "</strong>"...)
	case CodeSpanKind:
		dst = append(dst, "<code>"...)
		for _, c := range inline.children {
			dst = appendInlineHTML(dst, source, refMap, c)
		}
		dst = append(dst, "</code>"...)
	case LinkKind:
		var def LinkDefinition
		if ref := inline.LinkReference(); ref != "" {
			def = refMap[ref]
		} else {
			title := inline.LinkTitle()
			def = LinkDefinition{
				Destination:  inline.LinkDestination().Text(source),
				Title:        title.Text(source),
				TitlePresent: title != nil,
			}
		}
		dst = append(dst, `<a href="`...)
		dst = append(dst, html.EscapeString(NormalizeURI(def.Destination))...)
		dst = append(dst, `"`...)
		if def.TitlePresent {
			dst = append(dst, ` title="`...)
			dst = append(dst, html.EscapeString(def.Title)...)
			dst = append(dst, `"`...)
		}
		dst = append(dst, ">"...)
		for _, c := range inline.children {
			dst = appendInlineHTML(dst, source, refMap, c)
		}
		dst = append(dst, "</a>"...)
	case IndentKind:
		for i, n := 0, inline.IndentWidth(); i < n; i++ {
			dst = append(dst, ' ')
		}
	}
	return dst
}

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
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.ContainsRune(safeSet, c):
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
	return 'a' <= c && c <= 'f' || 'A' <= c && c <= 'f' || '0' <= c && c <= '9'
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
