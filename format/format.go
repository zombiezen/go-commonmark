// Copyright 2024 Ross Light
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

// Package format provides a function to format a Markdown file
// that is equivalent to the original Markdown.
package format

import (
	"bytes"
	"fmt"
	"io"

	"zombiezen.com/go/commonmark"
)

// Format writes the given blocks as CommonMark to the given writer.
func Format(w io.Writer, blocks []*commonmark.RootBlock) error {
	type stackFrame struct {
		*commonmark.Block
		source []byte
		indent int
	}

	stack := make([]stackFrame, 0, len(blocks))
	for i := len(blocks) - 1; i >= 0; i-- {
		stack = append(stack, stackFrame{
			source: blocks[i].Source,
			Block:  &blocks[i].Block,
		})
	}

	ww := &errWriter{w: w}
	var prevKind commonmark.BlockKind
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch k := curr.Kind(); k {
		case commonmark.ParagraphKind:
			if prevKind != 0 {
				ww.WriteString("\n")
			}
			formatInlines(ww, curr.source, curr.indent, curr.Block)
			prevKind = commonmark.ParagraphKind
		case commonmark.ThematicBreakKind:
			if prevKind == 0 {
				// Disambiguate from front matter.
				ww.WriteString("***\n\n")
			} else {
				ww.WriteString("\n---\n\n")
			}
			prevKind = commonmark.ThematicBreakKind
		case commonmark.ListKind:
			if prevKind != 0 && curr.IsTightList() {
				// Individual list items won't contain a blank line,
				// so add them beforehand.
				ww.WriteString("\n")
			}
			for i := curr.ChildCount() - 1; i >= 0; i-- {
				stack = append(stack, stackFrame{
					Block:  curr.Child(i).Block(),
					source: curr.source,
					indent: curr.indent,
				})
			}
		case commonmark.ListItemKind:
			if prevKind != 0 && !curr.IsTightList() {
				ww.WriteString("\n")
			}
			start := 0
			extraIndent := 0
			if marker := curr.Child(start).Block(); marker.Kind() == commonmark.ListMarkerKind {
				start++
				markerBytes := spanSlice(curr.source, marker.Span())
				ww.Write(markerBytes)
				ww.WriteString(" ")
				extraIndent = len(markerBytes) + 1
				prevKind = commonmark.ListItemKind
			}
			if curr.IsTightList() && curr.ChildCount()-start == 1 && curr.Child(start).Block().Kind() == commonmark.ParagraphKind {
				content := spanSlice(curr.source, curr.Child(start).Span())
				// TODO(now): get last byte written
				formatInlines(ww, curr.source, curr.indent+extraIndent, curr.Child(start).Block())
				if !bytes.HasSuffix(content, []byte("\n")) {
					ww.WriteString("\n")
				}
				prevKind = commonmark.ListItemKind
				continue
			}
			for i := curr.ChildCount() - 1; i >= start; i-- {
				stack = append(stack, stackFrame{
					source: curr.source,
					Block:  curr.Child(i).Block(),
					indent: curr.indent + extraIndent,
				})
			}
		case commonmark.LinkReferenceDefinitionKind:
			if prevKind != 0 {
				ww.WriteString("\n")
			}
			ww.WriteString("[")
			ww.WriteString(curr.Child(0).Inline().LinkReference())
			ww.WriteString("]: ")
			ww.WriteString(curr.Child(1).Inline().Text(curr.source))
			if curr.ChildCount() > 2 {
				ww.WriteString(` "`)
				ww.WriteString(curr.Child(2).Inline().Text(curr.source))
				ww.WriteString(`"`)
			}
			ww.WriteString("\n")
		default:
			return fmt.Errorf("format commonmark: unhandled block kind %v", k)
		}
	}
	return ww.err
}

func formatInlines(w *errWriter, source []byte, indent int, block *commonmark.Block) {
	for i := 0; i < block.ChildCount(); i++ {
		child := block.Child(i).Inline()
		switch child.Kind() {
		case commonmark.LinkKind:
			w.WriteString("[")
			for j := 0; j < child.ChildCount(); j++ {
				linkChild := child.Child(j)
				if k := linkChild.Kind(); k != commonmark.LinkDestinationKind && k != commonmark.LinkTitleKind && k != commonmark.LinkLabelKind {
					w.Write(spanSlice(source, linkChild.Span()))
				}
			}
			w.WriteString("]")
			if ref := child.LinkReference(); ref != "" {
				if isShortcutLinkOrImage(child) {
					// Turn shortcut links into collapsed links.
					w.WriteString("[]")
				} else {
					w.WriteString("[")
					w.WriteString(ref)
					w.WriteString("]")
				}
			} else {
				dst := child.LinkDestination()
				title := child.LinkTitle()
				if dst != nil || title != nil {
					w.WriteString("(")
					if dst != nil {
						w.WriteString(commonmark.NormalizeURI(dst.Text(source)))
						if title != nil {
							w.WriteString(" ")
						}
					}
					if title != nil {
						w.WriteString(`"`)
						w.WriteString(title.Text(source))
						w.WriteString(`"`)
					}
					w.WriteString(")")
				}
			}
		default:
			if child.Span().IsValid() {
				indentedWrite(w, indent, spanSlice(source, child.Span()))
			}
		}
	}
	w.WriteString("\n")
}

func indentedWrite(w *errWriter, indent int, p []byte) {
	for {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			break
		}
		w.Write(p[:i+1])
		for j := 0; j < indent; j++ {
			w.WriteString(" ")
		}
		p = p[i+1:]
	}
	w.Write(p)
}

func isShortcutLinkOrImage(inline *commonmark.Inline) bool {
	if k := inline.Kind(); k != commonmark.LinkKind && k != commonmark.ImageKind || inline.ChildCount() == 0 {
		return false
	}
	last := inline.Child(inline.ChildCount() - 1)
	if last.Kind() == commonmark.LinkLabelKind {
		return false
	}
	for i := inline.ChildCount() - 1; i >= inline.ChildCount()-2 && i >= 0; i-- {
		if child := inline.Child(i); child.Kind() == commonmark.LinkDestinationKind {
			return false
		}
	}
	return true
}

type errWriter struct {
	w   io.Writer
	err error
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	n, w.err = w.w.Write(p)
	return n, w.err
}

func (w *errWriter) WriteString(s string) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	n, w.err = io.WriteString(w.w, s)
	return n, w.err
}

func spanSlice(b []byte, span commonmark.Span) []byte {
	return b[span.Start:span.End]
}
