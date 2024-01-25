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
	"io"
	"strings"

	"zombiezen.com/go/commonmark"
)

// Format writes the given blocks as CommonMark to the given writer.
func Format(w io.Writer, blocks []*commonmark.RootBlock) error {
	ww := &errWriter{w: w}
	for _, root := range blocks {
		indents := make(map[commonmark.Node]string)
		commonmark.Walk(root.AsNode(), &commonmark.WalkOptions{
			Pre: func(c *commonmark.Cursor) bool {
				if b := c.Node().Block(); b != nil {
					parentIndent := indents[c.Parent()]
					newIndent, ok := preBlock(ww, root.Source, parentIndent, c)
					indents[c.Node()] = parentIndent + newIndent
					return ok
				}
				if i := c.Node().Inline(); i != nil {
					return visitInline(ww, root.Source, indents[c.ParentBlock().AsNode()], c)
				}
				return false
			},
			Post: func(c *commonmark.Cursor) bool {
				if c.Node().Block() != nil {
					postBlock(ww, root.Source, c)
				}
				return true
			},
		})
	}
	return ww.err
}

func preBlock(w *errWriter, source []byte, indent string, cursor *commonmark.Cursor) (childrenIndent string, descend bool) {
	curr := cursor.Node().Block()
	switch k := curr.Kind(); k {
	case commonmark.ParagraphKind:
		if cursor.ParentBlock().IsTightList() {
			return "", true
		}
		if w.hasWritten {
			w.WriteString("\n")
		}
		return "", true
	case commonmark.ThematicBreakKind:
		if w.hasWritten {
			w.WriteString("\n---\n\n")
		} else {
			// Disambiguate from front matter.
			w.WriteString("***\n\n")
		}
		return "", true
	case commonmark.ListKind:
		if w.hasWritten && curr.IsTightList() {
			// Individual list items won't contain a blank line,
			// so add them beforehand.
			w.WriteString("\n")
		}
		return "", true
	case commonmark.ListItemKind:
		if w.hasWritten && !curr.IsTightList() {
			w.WriteString("\n")
		}
		start := 0
		if marker := curr.Child(start).Block(); marker.Kind() == commonmark.ListMarkerKind {
			start++
			markerBytes := spanSlice(source, marker.Span())
			w.Write(markerBytes)
			w.WriteString(" ")
			childrenIndent = strings.Repeat(" ", len(markerBytes)+1)
		}
		return childrenIndent, true
	case commonmark.LinkReferenceDefinitionKind:
		if w.hasWritten {
			w.WriteString("\n")
		}
		w.WriteString("[")
		w.WriteString(curr.Child(0).Inline().LinkReference())
		w.WriteString("]: ")
		w.WriteString(curr.Child(1).Inline().Text(source))
		if curr.ChildCount() > 2 {
			w.WriteString(` "`)
			w.WriteString(curr.Child(2).Inline().Text(source))
			w.WriteString(`"`)
		}
		w.WriteString("\n")
		return "", false
	default:
		return "", false
	}
}

func postBlock(w *errWriter, source []byte, cursor *commonmark.Cursor) {
	b := cursor.Node().Block()
	switch b.Kind() {
	case commonmark.ParagraphKind:
		if !cursor.ParentBlock().IsTightList() {
			w.WriteString("\n")
		}
	case commonmark.ListItemKind:
		w.WriteString("\n")
	}
}

func visitInline(w *errWriter, source []byte, indent string, cursor *commonmark.Cursor) bool {
	child := cursor.Node().Inline()
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
		return false
	default:
		if !child.Span().IsValid() {
			return false
		}
		indentedWrite(w, indent, spanSlice(source, child.Span()))
		return false
	}
}

func indentedWrite(w *errWriter, indent string, p []byte) {
	for {
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			break
		}
		w.Write(p[:i+1])
		w.WriteString(indent)
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
	w          io.Writer
	hasWritten bool
	err        error
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	n, w.err = w.w.Write(p)
	w.hasWritten = w.hasWritten || n > 0
	return n, w.err
}

func (w *errWriter) WriteString(s string) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	n, w.err = io.WriteString(w.w, s)
	w.hasWritten = w.hasWritten || n > 0
	return n, w.err
}

func spanSlice(b []byte, span commonmark.Span) []byte {
	return b[span.Start:span.End]
}
