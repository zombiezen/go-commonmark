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
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"zombiezen.com/go/commonmark"
)

// Format writes the given blocks as CommonMark to the given writer.
func Format(w io.Writer, blocks []*commonmark.RootBlock) error {
	fw := newFormatWriter(w)
	var source []byte
	commonmark.Walk(commonmark.Node{}, &commonmark.WalkOptions{
		Pre: func(c *commonmark.Cursor) bool {
			if b := c.Node().Block(); b != nil {
				if c.ParentBlock() == nil {
					for _, root := range blocks {
						if b == &root.Block {
							source = root.Source
							break
						}
					}
				}

				newIndent, ok := preBlock(fw, source, c)
				if ok {
					fw.push(newIndent)
				}
				return ok
			}
			if i := c.Node().Inline(); i != nil {
				return visitInline(fw, source, c)
			}
			return c.Node() == commonmark.Node{}
		},
		Post: func(c *commonmark.Cursor) bool {
			if c.Node().Block() != nil {
				fw.pop()
				postBlock(fw, source, c)
			}
			if c.Node().Inline() != nil {
				postInline(fw, source, c)
			}
			return true
		},
		ChildCount: func(n commonmark.Node) int {
			if n == (commonmark.Node{}) {
				return len(blocks)
			}
			return n.ChildCount()
		},
		Child: func(n commonmark.Node, i int) commonmark.Node {
			if n == (commonmark.Node{}) {
				return blocks[i].AsNode()
			}
			return n.Child(i)
		},
	})
	return fw.err
}

func preBlock(fw *formatWriter, source []byte, cursor *commonmark.Cursor) (childrenIndent string, descend bool) {
	curr := cursor.Node().Block()
	switch k := curr.Kind(); k {
	case commonmark.ParagraphKind:
		if !isFirstParagraph(cursor) {
			fw.s("\n")
		}
		return "", true
	case commonmark.ThematicBreakKind:
		if fw.hasWritten {
			fw.s("\n---\n\n")
		} else {
			// Disambiguate from front matter.
			fw.s("***\n\n")
		}
		return "", true
	case commonmark.ListKind:
		if fw.hasWritten && curr.IsTightList() {
			// Individual list items won't contain a blank line,
			// so add them beforehand.
			fw.s("\n")
		}
		return "", true
	case commonmark.ListItemKind:
		if cursor.Index() > 0 && !curr.IsTightList() {
			fw.s("\n")
		}
		start := 0
		if marker := curr.Child(start).Block(); marker.Kind() == commonmark.ListMarkerKind {
			start++
			markerBytes := spanSlice(source, marker.Span())
			fw.b(markerBytes)
			fw.s(" ")
			childrenIndent = strings.Repeat(" ", len(markerBytes)+1)
		}
		return childrenIndent, true
	case commonmark.LinkReferenceDefinitionKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		fw.s("[")
		fw.s(curr.Child(0).Inline().LinkReference())
		fw.s("]: ")
		fw.s(curr.Child(1).Inline().Text(source))
		if curr.ChildCount() > 2 {
			fw.s(` "`)
			fw.s(curr.Child(2).Inline().Text(source))
			fw.s(`"`)
		}
		fw.s("\n")
		return "", false
	case commonmark.BlockQuoteKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		fw.s("> ")
		return "> ", true
	case commonmark.IndentedCodeBlockKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		fw.s("```\n")
		return "", true
	case commonmark.FencedCodeBlockKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		fw.s("```")
		if info := curr.InfoString(); info != nil {
			fw.b(spanSlice(source, info.Span()))
		}
		fw.s("\n")
		return "", true
	case commonmark.ATXHeadingKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		for i, n := 0, curr.HeadingLevel(); i < n; i++ {
			fw.s("#")
		}
		fw.s(" ")
		return "", true
	case commonmark.SetextHeadingKind:
		if fw.hasWritten {
			fw.s("\n")
		}
		return "", true
	default:
		return "", false
	}
}

func isFirstParagraph(cursor *commonmark.Cursor) bool {
	if cursor.Node().Block().Kind() != commonmark.ParagraphKind {
		return false
	}
	if cursor.Index() <= 0 {
		return true
	}
	parent := cursor.Parent().Block()
	if cursor.Index() == 1 && parent.Kind() == commonmark.ListItemKind && parent.Child(0).Block().Kind() == commonmark.ListMarkerKind {
		return true
	}
	return false
}

func postBlock(fw *formatWriter, source []byte, cursor *commonmark.Cursor) {
	b := cursor.Node().Block()
	switch b.Kind() {
	case commonmark.ParagraphKind:
		if !cursor.ParentBlock().IsTightList() {
			fw.s("\n")
		}
	case commonmark.ListItemKind:
		fw.s("\n")
	case commonmark.IndentedCodeBlockKind, commonmark.FencedCodeBlockKind:
		fw.s("```\n")
	case commonmark.ATXHeadingKind:
		fw.s("\n")
	case commonmark.SetextHeadingKind:
		// TODO(someday): Extend to the length of the source.
		if b.HeadingLevel() == 1 {
			fw.s("\n=====\n")
		} else {
			fw.s("\n-----\n")
		}
	}
}

func visitInline(fw *formatWriter, source []byte, cursor *commonmark.Cursor) bool {
	child := cursor.Node().Inline()
	switch child.Kind() {
	case commonmark.LinkKind:
		fw.s("[")
		return true
	case commonmark.TextKind:
		if cursor.ParentBlock().Kind().IsCode() {
			fw.b(spanSlice(source, child.Span()))
			return false
		}

		for s := spanSlice(source, child.Span()); len(s) > 0; {
			r, n := utf8.DecodeRune(s)
			if strings.ContainsRune(`\[]*_<&`+"`", r) {
				fw.s(`\`)
			}
			fw.b(s[:n])
			s = s[n:]
		}
		return false
	case commonmark.InfoStringKind, commonmark.LinkDestinationKind, commonmark.LinkLabelKind, commonmark.LinkTitleKind:
		return false
	default:
		if !child.Span().IsValid() {
			return false
		}
		fw.b(spanSlice(source, child.Span()))
		return false
	}
}

func postInline(fw *formatWriter, source []byte, cursor *commonmark.Cursor) {
	child := cursor.Node().Inline()
	switch child.Kind() {
	case commonmark.LinkKind:
		fw.s("]")
		if ref := child.LinkReference(); ref != "" {
			if isShortcutLinkOrImage(child) {
				// Turn shortcut links into collapsed links.
				fw.s("[]")
			} else {
				fw.s("[")
				fw.s(ref)
				fw.s("]")
			}
		} else {
			fw.s("(")
			title := child.LinkTitle()
			if dst := child.LinkDestination(); dst != nil {
				fw.s(commonmark.NormalizeURI(dst.Text(source)))
				if title != nil {
					fw.s(" ")
				}
			}
			if title != nil {
				fw.s(`"`)
				fw.s(title.Text(source))
				fw.s(`"`)
			}
			fw.s(")")
		}
	}
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

type formatWriter struct {
	w           stringWriter
	indents     []string
	startedLine bool

	hasWritten bool
	err        error
}

func newFormatWriter(w io.Writer) *formatWriter {
	sw, ok := w.(stringWriter)
	if !ok {
		return &formatWriter{w: fallbackStringWriter{w}}
	}
	return &formatWriter{w: sw}
}

func (fw *formatWriter) push(indent string) {
	fw.indents = append(fw.indents, indent)
}

func (fw *formatWriter) pop() {
	fw.indents = fw.indents[:len(fw.indents)-1]
}

func (fw *formatWriter) b(p []byte) {
	// TODO(soon): Reimplement to avoid allocations.
	fw.s(string(p))
}

func (fw *formatWriter) s(s string) {
	if fw.err != nil {
		return
	}

	for {
		i := strings.IndexByte(s, '\n')
		if i == -1 {
			break
		}
		fw.hasWritten = true
		if !fw.startedLine {
			if i == 0 {
				// For blank lines: don't leave trailing whitespace.
				if fw.err = writeTrimmedIndent(fw.w, fw.indents); fw.err != nil {
					return
				}
				if _, fw.err = fw.w.WriteString("\n"); fw.err != nil {
					return
				}
				s = s[1:]
				continue
			}

			if fw.err = writeStrings(fw.w, fw.indents); fw.err != nil {
				return
			}
		}

		if _, fw.err = fw.w.WriteString(s[:i+1]); fw.err != nil {
			return
		}
		fw.startedLine = false
		s = s[i+1:]
	}

	if len(s) == 0 {
		return
	}
	fw.hasWritten = true
	if !fw.startedLine {
		if fw.err = writeStrings(fw.w, fw.indents); fw.err != nil {
			return
		}
	}
	_, fw.err = fw.w.WriteString(s)
	fw.startedLine = true
}

func writeStrings(w io.StringWriter, slice []string) error {
	for _, s := range slice {
		if _, err := w.WriteString(s); err != nil {
			return err
		}
	}
	return nil
}

func writeTrimmedIndent(w io.StringWriter, indents []string) error {
	var lastLen int
	for {
		if len(indents) == 0 {
			return nil
		}
		last := indents[len(indents)-1]
		i := strings.LastIndexFunc(last, func(r rune) bool {
			return !unicode.IsSpace(r)
		})
		if i >= 0 {
			_, n := utf8.DecodeRuneInString(last[i:])
			lastLen = i + n
			break
		}
		indents = indents[:len(indents)-1]
	}
	if err := writeStrings(w, indents[:len(indents)-1]); err != nil {
		return err
	}
	_, err := w.WriteString(indents[len(indents)-1][:lastLen])
	return err
}

type stringWriter interface {
	io.Writer
	io.StringWriter
}

type fallbackStringWriter struct {
	io.Writer
}

func (sw fallbackStringWriter) WriteString(s string) (n int, err error) {
	return sw.Writer.Write([]byte(s))
}

func spanSlice(b []byte, span commonmark.Span) []byte {
	return b[span.Start:span.End]
}
