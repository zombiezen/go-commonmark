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
)

func RenderHTML(w io.Writer, blocks []*RootBlock) error {
	var buf []byte
	for i, b := range blocks {
		buf = buf[:0]
		if i > 0 {
			buf = append(buf, "\n\n"...)
		}
		buf = appendHTML(buf, b.Source, &b.Block)
		if _, err := w.Write(buf); err != nil {
			return fmt.Errorf("render markdown to html: %w", err)
		}
	}
	return nil
}

func appendHTML(dst []byte, source []byte, block *Block) []byte {
	switch block.Kind() {
	case ParagraphKind:
		dst = append(dst, "<p>"...)
		dst = appendChildrenHTML(dst, source, block.Children(), false)
		dst = append(dst, "</p>"...)
	case ThematicBreakKind:
		dst = append(dst, "<hr>"...)
	case ATXHeadingKind, SetextHeadingKind:
		level := block.HeadingLevel()
		dst = append(dst, "<h"...)
		dst = strconv.AppendInt(dst, int64(level), 10)
		dst = append(dst, ">"...)
		dst = appendChildrenHTML(dst, source, block.Children(), false)
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
		dst = appendChildrenHTML(dst, source, block.Children(), false)
		dst = append(dst, "</code></pre>"...)
	case BlockQuoteKind:
		dst = append(dst, "<blockquote>"...)
		dst = appendChildrenHTML(dst, source, block.Children(), false)
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
		dst = appendChildrenHTML(dst, source, block.Children(), false)
		if block.IsOrderedList() {
			dst = append(dst, "</ol>"...)
		} else {
			dst = append(dst, "</ul>"...)
		}
	case ListItemKind:
		dst = append(dst, "<li>"...)
		dst = appendChildrenHTML(dst, source, block.Children(), block.IsTightList())
		dst = append(dst, "</li>"...)
	}
	return dst
}

func appendChildrenHTML(dst []byte, source []byte, children []Node, tight bool) []byte {
	for _, c := range children {
		if inline := c.Inline(); inline != nil {
			dst = appendInlineHTML(dst, source, inline)
			continue
		}
		if sub := c.Block(); sub != nil {
			if tight && sub.Kind() == ParagraphKind {
				dst = appendChildrenHTML(dst, source, sub.Children(), false)
			} else {
				dst = appendHTML(dst, source, sub)
			}
			continue
		}
	}
	return dst
}

func appendInlineHTML(dst []byte, source []byte, inline *Inline) []byte {
	switch inline.Kind() {
	case TextKind, UnparsedKind:
		dst = append(dst, html.EscapeString(string(source[inline.Start():inline.End()]))...)
	case SoftLineBreakKind:
		dst = append(dst, '\n')
	case HardLineBreakKind:
		dst = append(dst, "<br>\n"...)
	case IndentKind:
		for i, n := 0, inline.IndentWidth(); i < n; i++ {
			dst = append(dst, ' ')
		}
	}
	return dst
}
