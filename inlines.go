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
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
)

// Inline represents CommonMark content elements like text, links, or emphasis.
type Inline struct {
	kind     InlineKind
	span     Span
	indent   int
	ref      string
	children []*Inline
}

// Kind returns the type of inline node
// or zero if the node is nil.
func (inline *Inline) Kind() InlineKind {
	if inline == nil {
		return 0
	}
	return InlineKind(inline.kind)
}

// Span returns the position information relative to the [RootBlock]'s Source field.
func (inline *Inline) Span() Span {
	if inline == nil {
		return NullSpan()
	}
	return inline.span
}

// IndentWidth returns the number of spaces the [IndentKind] span represents,
// or zero if the node is nil or of a different type.
func (inline *Inline) IndentWidth() int {
	if inline == nil {
		return 0
	}
	return inline.indent
}

// Text converts a non-container inline node into a string.
func (inline *Inline) Text(source []byte) string {
	switch inline.Kind() {
	case TextKind, RawHTMLKind:
		return string(spanSlice(source, inline.Span()))
	case CharacterReferenceKind:
		return html.UnescapeString(string(spanSlice(source, inline.Span())))
	case SoftLineBreakKind, HardLineBreakKind:
		return "\n"
	case IndentKind:
		sb := new(strings.Builder)
		for i := 0; i < inline.IndentWidth(); i++ {
			sb.WriteByte(' ')
		}
		return sb.String()
	case InfoStringKind, LinkDestinationKind, LinkTitleKind:
		sb := new(strings.Builder)
		sb.Grow(inline.Span().Len())
		for i, n := 0, inline.ChildCount(); i < n; i++ {
			switch child := inline.Child(i); child.Kind() {
			case TextKind:
				sb.Write(spanSlice(source, child.Span()))
			case CharacterReferenceKind:
				sb.WriteString(html.UnescapeString(string(spanSlice(source, child.Span()))))
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// LinkDestination returns the destination child of a [LinkKind] node
// or nil if none is present or the node is not a link.
func (inline *Inline) LinkDestination() *Inline {
	if k := inline.Kind(); k != LinkKind && k != ImageKind {
		return nil
	}
	for i := len(inline.children) - 1; i >= len(inline.children)-2 && i >= 0; i-- {
		if child := inline.children[i]; child.Kind() == LinkDestinationKind {
			return child
		}
	}
	return nil
}

// LinkTitle returns the title child of a [LinkKind] node
// or nil if none is present or the node is not a link.
func (inline *Inline) LinkTitle() *Inline {
	if k := inline.Kind(); k != LinkKind && k != ImageKind {
		return nil
	}
	for i := len(inline.children) - 1; i >= len(inline.children)-2 && i >= 0; i-- {
		if child := inline.children[i]; child.Kind() == LinkTitleKind {
			return child
		}
	}
	return nil
}

// LinkReference returns the [normalized form] of a link label.
//
// [normalized form]: https://spec.commonmark.org/0.30/#matches
func (inline *Inline) LinkReference() string {
	if k := inline.Kind(); (k == LinkKind || k == ImageKind) && len(inline.children) > 0 {
		if last := inline.children[len(inline.children)-1]; last.Kind() == LinkLabelKind {
			// Full reference link.
			return last.LinkReference()
		}
	}
	return inline.ref
}

func transformLinkReference(source []byte, nodes []*Inline) string {
	if len(nodes) == 0 {
		return ""
	}
	return transformLinkReferenceSpan(source, nodes, Span{
		Start: nodes[0].Span().Start,
		End:   nodes[len(nodes)-1].Span().End,
	})
}

func transformLinkReferenceSpan(source []byte, nodes []*Inline, span Span) string {
	sb := new(strings.Builder)
	r := newInlineByteReader(source, nodes, span.Start)
	for r.pos < span.End {
		c := r.current()
		if isSpaceTabOrLineEnding(c) {
			// Collapse consecutive whitespace to a single space.
			sb.WriteByte(' ')
			if !r.next() {
				break
			}
			for r.pos < span.End && isSpaceTabOrLineEnding(r.current()) {
				if !r.next() {
					break
				}
			}
		} else {
			sb.WriteByte(c)
			if !r.next() {
				break
			}
		}
	}
	return cases.Fold().String(strings.TrimSpace(sb.String()))
}

// ChildCount returns the number of children the node has.
// Calling ChildCount on nil returns 0.
func (inline *Inline) ChildCount() int {
	if inline == nil {
		return 0
	}
	return len(inline.children)
}

// Child returns the i'th child of the node.
func (inline *Inline) Child(i int) *Inline {
	return inline.children[i]
}

// InlineKind is an enumeration of values returned by [*Inline.Kind].
type InlineKind uint16

const (
	// TextKind is used for literal text.
	TextKind InlineKind = 1 + iota
	// SoftLineBreakKind is rendered as either a space or as a hard line break,
	// depending on the renderer.
	SoftLineBreakKind
	// HardLineBreakKind is rendered as a line break.
	HardLineBreakKind
	// IndentKind represents one or more space characters
	// (the exact number can be retrieved by [*Inline.IndentWidth]).
	// It's placed in the parse tree
	// in situations where the number of logical spaces does not match the source.
	IndentKind
	// CharacterReferenceKind is used for ampersand escape characters
	// (e.g. "&amp;").
	CharacterReferenceKind
	// InfoStringKind is used for the [info string] of a fenced code block.
	// It's typically not rendered directly and its contents are implementation-defined.
	//
	// [info string]: https://spec.commonmark.org/0.30/#info-string
	InfoStringKind
	// EmphasisKind is used for text that has stress emphasis.
	EmphasisKind
	// StrongKind is used for text that has strong emphasis.
	StrongKind
	// LinkKind is used for hyperlinks.
	// The [*Inline.LinkDestination], [*Inline.LinkTitle], and [*Inline.LinkReference] methods
	// can be used to retrieve specific parts of the link.
	LinkKind
	// ImageKind is used for images.
	// The contents of the node are used as the image's text description.
	// Otherwise, ImageKind is similar to [LinkKind].
	ImageKind
	// LinkDestinationKind is used as part of links and images
	// to indicate the destination or image source, respectively.
	LinkDestinationKind
	// LinkTitleKind is used as part of links and images
	// to hold advisory text typically rendered as a tooltip.
	LinkTitleKind
	// LinkLabelKind is used as either a link reference definition label
	// or in a link or image to reference a link reference definition.
	LinkLabelKind
	// CodeSpanKind is used for inline code in a non-code-block context.
	CodeSpanKind
	// AutolinkKind is used for [autolinks].
	// The node's content is also the link's destination.
	//
	// [autolinks]: https://spec.commonmark.org/0.30/#autolinks
	AutolinkKind

	// HTMLTagKind is a container for one or more [RawHTMLKind] nodes
	// that represents an open tag, a closing tag, an HTML comment,
	// a processing instruction, a declaration, or a CDATA section.
	HTMLTagKind
	// RawHTMLKind is a text node that should be reproduced in HTML verbatim.
	RawHTMLKind

	// UnparsedKind is used for inline text that has not been tokenized.
	UnparsedKind
)

// An InlineParser converts [UnparsedKind] [Inline] nodes
// into inline trees.
type InlineParser struct {
	ReferenceMatcher ReferenceMatcher
}

// Rewrite replaces any [UnparsedKind] nodes in the given root block
// with parsed versions of the node.
func (p *InlineParser) Rewrite(root *RootBlock) {
	stack := []*Block{&root.Block}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch {
		case len(curr.inlineChildren) > 0 && hasUnparsed(curr):
			curr.inlineChildren = p.parse(root.Source, curr)
		case len(curr.blockChildren) > 0:
			for i := len(curr.blockChildren) - 1; i >= 0; i-- {
				if b := curr.blockChildren[i]; b != nil {
					stack = append(stack, b)
				}
			}
		}
	}
}

type inlineState struct {
	root             *Inline
	source           []byte
	unparsed         []*Inline
	unparsedPos      int
	blockKind        BlockKind
	stack            []delimiterStackElement
	ignoreNextIndent bool
	parentMap        map[*Inline]*Inline
}

func (state *inlineState) spanEnd() int {
	if state.unparsedPos >= len(state.unparsed) {
		return len(state.source)
	}
	return state.unparsed[state.unparsedPos].Span().End
}

func (state *inlineState) isLastSpan() bool {
	return state.unparsedPos >= len(state.unparsed)-1
}

func (p *InlineParser) parse(source []byte, container *Block) []*Inline {
	dummy := &Inline{
		span: container.span,
	}
	state := &inlineState{
		root:      dummy,
		source:    source,
		blockKind: container.Kind(),
		unparsed:  container.inlineChildren,
		parentMap: make(map[*Inline]*Inline),
	}
	for ; state.unparsedPos < len(state.unparsed); state.unparsedPos++ {
		switch state.unparsed[state.unparsedPos].Kind() {
		case 0:
			state.ignoreNextIndent = false
		case IndentKind:
			if !state.ignoreNextIndent {
				dummy.children = append(dummy.children, state.unparsed[state.unparsedPos])
			}
		case UnparsedKind:
			pos := state.unparsed[state.unparsedPos].Span().Start
			if state.ignoreNextIndent {
				for pos < state.spanEnd() && (source[pos] == ' ' || source[pos] == '\t') {
					pos++
				}
			}
			state.ignoreNextIndent = false
			plainStart := pos
			for state.unparsedPos < len(state.unparsed) && pos < state.spanEnd() {
				switch source[pos] {
				case '*', '_':
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					pos = p.parseDelimiterRun(state, pos)
					plainStart = pos
				case '[':
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					node := &Inline{
						kind: TextKind,
						span: Span{
							Start: pos,
							End:   pos + 1,
						},
					}
					state.addToRoot(node)
					state.stack = append(state.stack, delimiterStackElement{
						typ:   inlineDelimiterLink,
						flags: activeFlag,
						node:  node,
					})
					pos++
					plainStart = pos
				case ']':
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					pos = p.parseEndBracket(state, pos)
					plainStart = pos
				case '!':
					if pos+1 >= state.spanEnd() || source[pos+1] != '[' {
						pos++
						continue
					}
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					node := &Inline{
						kind: TextKind,
						span: Span{
							Start: pos,
							End:   pos + 2,
						},
					}
					state.addToRoot(node)
					state.stack = append(state.stack, delimiterStackElement{
						typ:   inlineDelimiterImage,
						flags: activeFlag,
						node:  node,
					})
					pos += 2
					plainStart = pos
				case ' ':
					end, ok := parseHardLineBreakSpace(source[pos:state.spanEnd()])
					if ok && !state.isLastSpan() {
						state.addToRoot(&Inline{
							kind: TextKind,
							span: Span{
								Start: plainStart,
								End:   pos,
							},
						})
						state.addToRoot(&Inline{
							kind: HardLineBreakKind,
							span: Span{
								Start: pos,
								End:   pos + end,
							},
						})
						// Leading spaces at the beginning of the next line are ignored.
						state.ignoreNextIndent = true
						plainStart = pos + end
					}
					pos += end
				case '`':
					if cs := p.parseCodeSpan(state, pos); cs.span.IsValid() {
						state.addToRoot(&Inline{
							kind: TextKind,
							span: Span{
								Start: plainStart,
								End:   cs.span.Start,
							},
						})
						p.collectCodeSpan(state, cs)

						pos = cs.span.End
						plainStart = pos
					} else {
						// Advance past literal backtick string.
						pos = cs.content.Start
					}
				case '<':
					if end := parseAutolink(state.source[pos:state.spanEnd()]); end >= 0 {
						end += pos
						state.addToRoot(&Inline{
							kind: TextKind,
							span: Span{
								Start: plainStart,
								End:   pos,
							},
						})
						state.addToRoot(&Inline{
							kind: AutolinkKind,
							span: Span{
								Start: pos,
								End:   end,
							},
							children: []*Inline{{
								kind: TextKind,
								span: Span{
									Start: pos + 1,
									End:   end - 1,
								},
							}},
						})
						pos = end
						plainStart = pos
						continue
					}
					r := newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], pos)
					span := parseHTMLTag(r)
					if !span.IsValid() {
						// TODO(soon): Autolinks.
						pos++
						continue
					}
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   span.Start,
						},
					})
					newNode := &Inline{
						kind: HTMLTagKind,
						span: span,
					}
					r = newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], span.Start)
					collectRawHTML(newNode, r, span.End)
					state.addToRoot(newNode)

					pos = span.End
					plainStart = pos
					if i := nodeIndexForPosition(state.unparsed[state.unparsedPos:], pos); i >= 0 {
						state.unparsedPos += i
					} else {
						state.unparsedPos = len(state.unparsed)
					}
				case '\\':
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					pos = p.parseBackslash(state, pos)
					plainStart = pos
				case '&':
					end := parseCharacterEscape(state.source[pos:state.spanEnd()])
					if end < 0 {
						pos++
						continue
					}
					state.addToRoot(&Inline{
						kind: TextKind,
						span: Span{
							Start: plainStart,
							End:   pos,
						},
					})
					state.addToRoot(&Inline{
						kind: CharacterReferenceKind,
						span: Span{
							Start: pos,
							End:   pos + end,
						},
					})
					pos += end
					plainStart = pos
				default:
					pos++
				}
			}
			state.addToRoot(&Inline{
				kind: TextKind,
				span: Span{
					Start: plainStart,
					End:   state.spanEnd(),
				},
			})
		default:
			state.ignoreNextIndent = false
			dummy.children = append(dummy.children, state.unparsed[state.unparsedPos])
		}
	}
	p.processEmphasis(state, 0)
	return dummy.children
}

func (p *InlineParser) parseBackslash(state *inlineState, start int) (end int) {
	if start+1 >= state.spanEnd() || state.source[start+1] == '\n' || state.source[start+1] == '\r' {
		// At end of line.
		newNode := &Inline{
			kind: HardLineBreakKind,
			span: Span{
				Start: start,
				End:   start + 1,
			},
		}
		if state.isLastSpan() {
			// Hard line breaks not permitted at end of block.
			newNode.kind = TextKind
		} else {
			// Leading spaces at the beginning of the next line are ignored.
			state.ignoreNextIndent = true
		}
		state.addToRoot(newNode)
		return newNode.Span().End
	}
	if isASCIIPunctuation(state.source[start+1]) {
		start++
		end = start + 1
		state.addToRoot(&Inline{
			kind: TextKind,
			span: Span{
				Start: start,
				End:   end,
			},
		})
		return end
	}
	end = start + 2
	state.addToRoot(&Inline{
		kind: TextKind,
		span: Span{
			Start: start,
			End:   end,
		},
	})
	return end
}

func parseCharacterEscape(text []byte) (end int) {
	if len(text) < 3 || text[0] != '&' {
		return -1
	}
	if text[1] != '#' {
		// Entity reference.
		for i, c := range text[1:] {
			switch {
			case c == ';':
				if i == 0 || !isEntity(text[:i+2]) {
					return -1
				}
				return i + 2
			case !isASCIILetter(c) && !isASCIIDigit(c):
				return -1
			}
		}
		return -1
	}

	if text[2] == 'x' || text[2] == 'X' {
		// Hexadecimal numeric character reference.
		const digitStart = 3
		const digitLimit = 6
		rest := text[digitStart:]
		if len(rest) > digitLimit+1 {
			rest = rest[:digitLimit+1]
		}
		for i, c := range rest {
			switch {
			case c == ';':
				if i == 0 {
					return -1
				}
				return digitStart + i + 1
			case !isHex(c):
				return -1
			}
		}
		return -1
	}

	// Decimal numeric character reference.
	const digitStart = 2
	const digitLimit = 7
	rest := text[digitStart:]
	if len(rest) > digitLimit+1 {
		rest = rest[:digitLimit+1]
	}
	for i, c := range rest {
		switch {
		case c == ';':
			if i == 0 {
				return -1
			}
			return digitStart + i + 1
		case !isASCIIDigit(c):
			return -1
		}
	}
	return -1
}

func isEntity(x []byte) bool {
	s := html.UnescapeString(string(x))
	return !strings.HasPrefix(s, "&") || !strings.HasSuffix(s, ";")
}

func (p *InlineParser) parseDelimiterRun(state *inlineState, start int) (end int) {
	node := &Inline{
		kind: TextKind,
		span: Span{
			Start: start,
			End:   start + 1,
		},
	}
	for node.span.End < state.spanEnd() && state.source[node.span.End] == state.source[node.span.Start] {
		node.span.End++
	}

	elem := delimiterStackElement{
		flags: activeFlag | emphasisFlags(state.source, node.Span()),
		n:     node.Span().Len(),
		node:  node,
	}
	if state.source[node.Span().Start] == '*' {
		elem.typ = inlineDelimiterStar
	} else {
		elem.typ = inlineDelimiterUnderscore
	}

	state.addToRoot(node)
	state.stack = append(state.stack, elem)
	return node.Span().End
}

func (p *InlineParser) parseEndBracket(state *inlineState, start int) (end int) {
	openDelimIndex := p.lookForLinkOrImage(state)
	if openDelimIndex < 0 {
		state.addToRoot(&Inline{
			kind: TextKind,
			span: Span{
				Start: start,
				End:   start + 1,
			},
		})
		return start + 1
	}
	kind := LinkKind
	if state.stack[openDelimIndex].typ == inlineDelimiterImage {
		kind = ImageKind
	}

	// Attempt as inline link first,
	// but fall back to shortcut reference link below.
	if start+1 < state.spanEnd() && state.source[start+1] == '(' {
		if info := p.parseInlineLink(state, start+1); info.span.IsValid() {
			linkNode := state.wrap(kind, state.stack[openDelimIndex].node, nil)
			linkNode.span = Span{
				Start: state.stack[openDelimIndex].node.span.Start,
				End:   info.span.End,
			}
			if info.destination.span.IsValid() {
				destNode := &Inline{
					kind: LinkDestinationKind,
					span: info.destination.span,
				}
				if info.destination.text.IsValid() {
					r := newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], info.destination.text.Start)
					collectLinkAttributeText(destNode, r, info.destination.text.End)
				}
				linkNode.children = append(linkNode.children, destNode)
			}
			if info.title.span.IsValid() {
				destNode := &Inline{
					kind: LinkTitleKind,
					span: info.title.span,
				}
				if info.title.text.IsValid() {
					r := newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], info.title.text.Start)
					collectLinkAttributeText(destNode, r, info.title.text.End)
				}
				linkNode.children = append(linkNode.children, destNode)
			}
			p.finishLink(state, kind, openDelimIndex)
			return info.span.End
		}
	}

	switch {
	case start+2 < state.spanEnd() && state.source[start+1] == '[' && state.source[start+2] == ']':
		// Collapsed reference link.

		// Since we're backtracking, we use the full state.unparsed rather than a slice.
		normalizedLabel := transformLinkReferenceSpan(state.source, state.unparsed, Span{
			Start: state.stack[openDelimIndex].node.Span().End,
			End:   start,
		})
		if p.ReferenceMatcher == nil || !p.ReferenceMatcher.MatchReference(normalizedLabel) {
			state.addToRoot(&Inline{
				kind: TextKind,
				span: Span{
					Start: start,
					End:   start + 3,
				},
			})
			state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
			return start + 3
		}

		linkNode := state.wrap(kind, state.stack[openDelimIndex].node, nil)
		linkNode.span = Span{
			Start: state.stack[openDelimIndex].node.span.Start,
			End:   start + 3,
		}
		linkNode.ref = normalizedLabel
		linkNode.span.End = start + 3
		p.finishLink(state, kind, openDelimIndex)
		return linkNode.span.End
	case start+1 < state.spanEnd() && state.source[start+1] == '[':
		// Full reference link.
		label := parseLinkLabel(newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], start+1))
		if !label.span.IsValid() {
			state.addToRoot(&Inline{
				kind: TextKind,
				span: Span{
					Start: start,
					End:   start + 1,
				},
			})
			state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
			return start + 1
		}
		inlineLabel := &Inline{
			kind: LinkLabelKind,
			span: label.span,
		}
		collectLinkLabelText(
			inlineLabel,
			newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], label.inner.Start),
			label.inner.End,
		)
		inlineLabel.ref = transformLinkReference(state.source, inlineLabel.children)
		if p.ReferenceMatcher == nil || !p.ReferenceMatcher.MatchReference(inlineLabel.ref) {
			state.addToRoot(&Inline{
				kind: TextKind,
				span: Span{
					Start: start,
					End:   start + 1,
				},
			})
			state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
			return start + 1
		}

		linkNode := state.wrap(kind, state.stack[openDelimIndex].node, nil)
		linkNode.children = append(linkNode.children, inlineLabel)
		linkNode.span = Span{
			Start: state.stack[openDelimIndex].node.span.Start,
			End:   label.span.End,
		}
		p.finishLink(state, kind, openDelimIndex)
		return linkNode.span.End
	default:
		// Shortcut reference link.

		// Since we're backtracking, we use the full state.unparsed rather than a slice.
		normalizedLabel := transformLinkReferenceSpan(state.source, state.unparsed, Span{
			Start: state.stack[openDelimIndex].node.Span().End,
			End:   start,
		})
		if p.ReferenceMatcher == nil || !p.ReferenceMatcher.MatchReference(normalizedLabel) {
			state.addToRoot(&Inline{
				kind: TextKind,
				span: Span{
					Start: start,
					End:   start + 1,
				},
			})
			state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
			return start + 1
		}

		linkNode := state.wrap(kind, state.stack[openDelimIndex].node, nil)
		linkNode.ref = normalizedLabel
		linkNode.span = Span{
			Start: state.stack[openDelimIndex].node.span.Start,
			End:   start + 1,
		}
		p.finishLink(state, kind, openDelimIndex)
		return linkNode.span.End
	}
}

func (p *InlineParser) finishLink(state *inlineState, kind InlineKind, openDelimIndex int) {
	p.processEmphasis(state, openDelimIndex+1)
	state.remove(state.stack[openDelimIndex].node)
	state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
	if kind == LinkKind {
		for i := range state.stack[:openDelimIndex] {
			if state.stack[i].typ == inlineDelimiterLink {
				state.stack[i].flags &^= activeFlag
			}
		}
	}
}

type inlineLinkInfo struct {
	span        Span
	destination linkDestination
	title       linkTitle
}

func (p *InlineParser) parseInlineLink(state *inlineState, start int) (result inlineLinkInfo) {
	// Skip initial opening parenthesis.
	r := newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], start+1)
	defer func() {
		// If we successfully parse, advance the spans we're considering.
		if result.span.IsValid() {
			if i := nodeIndexForPosition(state.unparsed[state.unparsedPos:], result.span.End-1); i >= 0 {
				state.unparsedPos += i
			} else {
				state.unparsedPos = len(state.unparsed)
			}
		}
	}()
	if !skipLinkSpace(r) {
		return inlineLinkInfo{
			span: NullSpan(),
			destination: linkDestination{
				span: NullSpan(),
				text: NullSpan(),
			},
			title: linkTitle{
				span: NullSpan(),
				text: NullSpan(),
			},
		}
	}

	result = inlineLinkInfo{
		span: Span{
			Start: start,
			End:   -1,
		},
	}
	result.destination = parseLinkDestination(r)
	if result.destination.span.IsValid() {
		if !skipLinkSpace(r) {
			return inlineLinkInfo{
				span: NullSpan(),
				destination: linkDestination{
					span: NullSpan(),
					text: NullSpan(),
				},
				title: linkTitle{
					span: NullSpan(),
					text: NullSpan(),
				},
			}
		}
	}
	result.title = parseLinkTitle(r)
	if result.title.span.IsValid() {
		if !skipLinkSpace(r) {
			return inlineLinkInfo{
				span: NullSpan(),
				destination: linkDestination{
					span: NullSpan(),
					text: NullSpan(),
				},
				title: linkTitle{
					span: NullSpan(),
					text: NullSpan(),
				},
			}
		}
	}
	if r.current() != ')' {
		return inlineLinkInfo{
			span: NullSpan(),
			destination: linkDestination{
				span: NullSpan(),
				text: NullSpan(),
			},
			title: linkTitle{
				span: NullSpan(),
				text: NullSpan(),
			},
		}
	}
	result.span.End = r.pos + 1
	return result
}

type linkDestination struct {
	span Span
	text Span
}

func parseLinkDestination(r *inlineByteReader) linkDestination {
	switch c := r.current(); {
	case c == '<':
		start := r.pos
		for r.next() {
			switch r.current() {
			case '\r', '\n':
				return linkDestination{span: NullSpan(), text: NullSpan()}
			case '\\':
				if !r.next() {
					return linkDestination{span: NullSpan(), text: NullSpan()}
				}
				if c := r.current(); c == '\n' || c == '\r' {
					return linkDestination{span: NullSpan(), text: NullSpan()}
				}
			case '>':
				r.next()
				return linkDestination{
					span: Span{
						Start: start,
						End:   r.prevPos + 1,
					},
					text: Span{
						Start: start + 1,
						End:   r.prevPos,
					},
				}
			}
		}
		return linkDestination{span: NullSpan(), text: NullSpan()}
	case !isASCIIControl(c) && c != ' ' && c != ')':
		parenCount := 0
		start := r.pos
	loop:
		for {
			switch c := r.current(); {
			case isASCIIControl(c) || c == ' ':
				break loop
			case c == '\\':
				if !r.next() {
					break loop
				}
				if c := r.current(); isASCIIControl(c) || c == ' ' {
					break loop
				}
			case c == '(':
				parenCount++
			case c == ')':
				parenCount--
				if parenCount < 0 {
					break loop
				}
			}
			if !r.next() {
				break
			}
		}
		span := Span{Start: start, End: r.pos}
		return linkDestination{span: span, text: span}
	default:
		return linkDestination{span: NullSpan(), text: NullSpan()}
	}
}

type linkTitle struct {
	span Span
	text Span
}

func parseLinkTitle(r *inlineByteReader) linkTitle {
	firstChar := r.current()
	if firstChar != '\'' && firstChar != '"' && firstChar != '(' {
		return linkTitle{span: NullSpan(), text: NullSpan()}
	}
	terminator := firstChar
	if firstChar == '(' {
		terminator = ')'
	}

	start := r.pos
	for r.next() {
		switch r.current() {
		case '\\':
			if !r.next() {
				return linkTitle{span: NullSpan(), text: NullSpan()}
			}
		case terminator:
			r.next()
			return linkTitle{
				span: Span{
					Start: start,
					End:   r.prevPos + 1,
				},
				text: Span{
					Start: start + 1,
					End:   r.prevPos,
				},
			}
		}
	}
	return linkTitle{span: NullSpan(), text: NullSpan()}
}

type linkLabel struct {
	span  Span
	inner Span
}

// parseLinkLabel parses a [link label].
//
// [link label]: https://spec.commonmark.org/0.30/#link-label
func parseLinkLabel(r *inlineByteReader) linkLabel {
	// "A link label can have at most 999 characters inside the square brackets."
	const maxChars = 999

	if r.current() != '[' {
		return linkLabel{NullSpan(), NullSpan()}
	}
	result := linkLabel{
		span:  Span{Start: r.pos, End: -1},
		inner: NullSpan(),
	}

	// Skip initial spaces.
	chars := 0
	for {
		if !r.next() {
			return linkLabel{NullSpan(), NullSpan()}
		}
		chars++
		c := r.current()
		if chars >= maxChars || c == '[' || c == ']' {
			return linkLabel{NullSpan(), NullSpan()}
		}
		if !isSpaceTabOrLineEnding(c) {
			break
		}
	}
	result.inner.Start = r.pos

	// Consume rest of the label text.
	for ; chars < maxChars && r.current() != '[' && r.current() != ']'; chars++ {
		if r.current() == '\\' {
			result.inner.End = r.pos + 1
			chars++
			if !r.next() {
				return linkLabel{NullSpan(), NullSpan()}
			}
			if !isSpaceTabOrLineEnding(r.current()) {
				result.inner.End = r.pos + 1
			}
		} else if !isSpaceTabOrLineEnding(r.current()) {
			result.inner.End = r.pos + 1
		}
		if !r.next() {
			return linkLabel{NullSpan(), NullSpan()}
		}
	}

	if r.current() != ']' {
		return linkLabel{NullSpan(), NullSpan()}
	}
	result.span.End = r.pos + 1
	r.next()
	return result
}

// skipLinkSpace skips over "spaces, tabs, and up to one line ending"
// (a frequent phrase in the CommonMark specification)
// and returns whether it stopped before EOF.
func skipLinkSpace(r *inlineByteReader) bool {
	// Even though the [inline link] spec says to only permit "up to one line ending",
	// this case is already handled for us by block parsing.
	//
	// [inline link]: https://spec.commonmark.org/0.30/#inline-link

	if r.current() == 0 {
		return false
	}
	for isSpaceTabOrLineEnding(r.current()) {
		if !r.next() {
			return false
		}
	}
	return true
}

func collectLinkAttributeText(parent *Inline, r *inlineByteReader, end int) {
	collectTextNodes(parent, r, end, TextKind, true)
}

func collectLinkLabelText(parent *Inline, r *inlineByteReader, end int) {
	collectTextNodes(parent, r, end, TextKind, false)
}

func collectRawHTML(parent *Inline, r *inlineByteReader, end int) {
	collectTextNodes(parent, r, end, RawHTMLKind, false)
}

func collectTextNodes(parent *Inline, r *inlineByteReader, end int, textKind InlineKind, escapes bool) {
	plainStart := r.pos
	for r.pos < end {
		curr := r.currentNode()
		if curr.Kind() == IndentKind {
			// Encountered an indent node.
			// Copy it over verbatim and skip it.
			if r.pos > plainStart {
				parent.children = append(parent.children, &Inline{
					kind: textKind,
					span: Span{
						Start: plainStart,
						End:   r.prevPos + 1,
					},
				})
			}
			parent.children = append(parent.children, curr)
			for r.next() && r.currentNode() == curr {
			}
			plainStart = r.pos
			continue
		}

		if escapes && curr.Kind() == UnparsedKind {
			switch r.current() {
			case '\\':
				if r.next() && r.pos < end && isASCIIPunctuation(r.current()) {
					if r.prevPos > plainStart {
						parent.children = append(parent.children, &Inline{
							kind: textKind,
							span: Span{
								Start: plainStart,
								End:   r.prevPos, // exclude backslash
							},
						})
					}
					plainStart = r.pos
				}
			case '&':
				if end := parseCharacterEscape(r.remainingNodeBytes()); end >= 0 {
					if r.pos > plainStart {
						parent.children = append(parent.children, &Inline{
							kind: textKind,
							span: Span{
								Start: plainStart,
								End:   r.pos,
							},
						})
					}
					parent.children = append(parent.children, &Inline{
						kind: CharacterReferenceKind,
						span: Span{
							Start: r.pos,
							End:   r.pos + end,
						},
					})
					plainStart = r.pos + end
					for i := 0; i < end-1; i++ {
						r.next()
					}
					if !r.next() {
						break
					}
					continue
				}
			}
		}

		if !r.next() {
			break
		}
		if r.jumped() {
			if r.prevPos > plainStart {
				parent.children = append(parent.children, &Inline{
					kind: textKind,
					span: Span{
						Start: plainStart,
						End:   r.prevPos + 1,
					},
				})
			}
			plainStart = r.pos
		}
	}

	if plainStart < end {
		parent.children = append(parent.children, &Inline{
			kind: textKind,
			span: Span{
				Start: plainStart,
				End:   end,
			},
		})
	}
}

func (p *InlineParser) lookForLinkOrImage(state *inlineState) int {
	for i := len(state.stack) - 1; i >= 0; i-- {
		curr := &state.stack[i]
		if curr.typ == inlineDelimiterLink || curr.typ == inlineDelimiterImage {
			if curr.flags&activeFlag == 0 {
				state.stack = deleteDelimiterStack(state.stack, i, i+1)
				return -1
			}
			return i
		}
	}
	return -1
}

// emphasisFlags determines whether the given [delimiter run]
// [can open emphasis] and/or [can close emphasis].
//
// [delimiter run]: https://spec.commonmark.org/0.30/#delimiter-run
// [can open emphasis]: https://spec.commonmark.org/0.30/#can-open-emphasis
// [can close emphasis]: https://spec.commonmark.org/0.30/#can-close-emphasis
func emphasisFlags(source []byte, span Span) uint8 {
	var flags uint8
	prevChar := ' '
	if span.Start > 0 {
		prevChar, _ = utf8.DecodeLastRune(source[:span.Start])
	}
	nextChar := ' '
	if span.End < len(source) {
		nextChar, _ = utf8.DecodeRune(source[span.End:])
	}
	leftFlanking := !isUnicodeWhitespace(nextChar) &&
		(!isUnicodePunctuation(nextChar) || isUnicodeWhitespace(prevChar) || isUnicodePunctuation(prevChar))
	rightFlanking := !isUnicodeWhitespace(prevChar) &&
		(!isUnicodePunctuation(prevChar) || isUnicodeWhitespace(nextChar) || isUnicodePunctuation(nextChar))
	if leftFlanking && (source[span.Start] == '*' || !rightFlanking || isUnicodePunctuation(prevChar)) {
		flags |= openerFlag
	}
	if rightFlanking && (source[span.Start] == '*' || !leftFlanking || isUnicodePunctuation(nextChar)) {
		flags |= closerFlag
	}
	return flags
}

// processEmphasis implements the [process emphasis procedure]
// to convert delimiters to emphasis spans.
//
// [process emphasis procedure]: https://spec.commonmark.org/0.30/#process-emphasis
func (p *InlineParser) processEmphasis(state *inlineState, stackBottom int) {
	currentPosition := stackBottom
	var openersBottom [openersBottomCount]int
	for i := range openersBottom {
		openersBottom[i] = stackBottom
	}
closerLoop:
	for {
		// Move current_position forward in the delimiter stack (if needed)
		// until we find the first potential closer with delimiter * or _.
		for {
			if currentPosition >= len(state.stack) {
				break closerLoop
			}
			if (state.stack[currentPosition].typ == inlineDelimiterStar ||
				state.stack[currentPosition].typ == inlineDelimiterUnderscore) &&
				state.stack[currentPosition].flags&closerFlag != 0 {
				break
			}
			currentPosition++
		}

		// Now, look back in the stack
		// (staying above stack_bottom and the openers_bottom for this delimiter type)
		// for the first matching potential opener ("matching" means same delimiter).
		openerIndex := currentPosition - 1
		openersBottomIndex := state.stack[currentPosition].openersBottomIndex()
		for openerIndex >= openersBottom[openersBottomIndex] &&
			!isEmphasisDelimiterMatch(state.stack[openerIndex], state.stack[currentPosition]) {
			openerIndex--
		}
		if openerIndex >= openersBottom[openersBottomIndex] {
			opener := state.stack[openerIndex].node
			closer := state.stack[currentPosition].node
			strong := opener.Span().Len() >= 2 && closer.Span().Len() >= 2
			if strong {
				opener.span.End -= 2
				closer.span.Start += 2
				state.wrap(StrongKind, opener, closer)
			} else {
				opener.span.End--
				closer.span.Start++
				state.wrap(EmphasisKind, opener, closer)
			}

			// Remove any delimiters between the opener and closer from the delimiter stack.
			state.stack = deleteDelimiterStack(state.stack, openerIndex+1, currentPosition)
			currentPosition = openerIndex + 1

			// If either the opening or the closing text nodes became empty,
			// remove them from the tree.
			if opener.Span().Len() == 0 {
				state.remove(opener)
				state.stack = deleteDelimiterStack(state.stack, openerIndex, openerIndex+1)
				currentPosition--
			}
			if closer.Span().Len() == 0 {
				state.remove(closer)
				state.stack = deleteDelimiterStack(state.stack, currentPosition, currentPosition+1)
			}
		} else {
			// We know that there are no openers for this kind of closer up to and including this point,
			// so put a lower bound on future searches.
			openersBottom[openersBottomIndex] = currentPosition

			if state.stack[currentPosition].flags&openerFlag == 0 {
				// Remove delimiter from the stack
				// since we know it can't be a closer either.
				copy(state.stack[currentPosition:], state.stack[currentPosition+1:])
				state.stack[len(state.stack)-1] = delimiterStackElement{}
				state.stack = state.stack[:len(state.stack)-1]
			} else {
				currentPosition++
			}
		}
	}

	// After we’re done, we remove all delimiters above stack_bottom from the delimiter stack.
	state.stack = deleteDelimiterStack(state.stack, stackBottom, len(state.stack))
}

type codeSpan struct {
	span    Span
	content Span
}

func (p *InlineParser) parseCodeSpan(state *inlineState, start int) codeSpan {
	result := codeSpan{
		span:    Span{Start: start, End: -1},
		content: Span{Start: start, End: -1},
	}
	backtickLength := 0
	r := newInlineByteReader(state.source, state.unparsed[state.unparsedPos:], start)
	for r.current() == '`' {
		backtickLength++
		ok := r.next()
		result.content.Start = r.pos
		if !ok {
			return result
		}
	}

	for {
		if r.current() != '`' {
			if !r.next() {
				return result
			}
			continue
		}
		currentRunLength := 1
		potentialEnd := r.pos
		for r.next() && r.current() == '`' {
			currentRunLength++
		}
		if currentRunLength == backtickLength {
			result.content.End = potentialEnd
			result.span.End = r.prevPos + 1
			return result
		}

		if !r.next() {
			return result
		}
	}
}

func (p *InlineParser) collectCodeSpan(state *inlineState, cs codeSpan) {
	codeSpanNode := &Inline{
		kind: CodeSpanKind,
		span: cs.span,
	}
	addSpan := func(child *Inline) {
		spanText := spanSlice(state.source, child.Span())
		trim := 0
		switch {
		case len(spanText) >= 2 && spanText[len(spanText)-2] == '\r' && spanText[len(spanText)-1] == '\n':
			trim = 2
		case len(spanText) >= 1 && (spanText[len(spanText)-1] == '\n' || spanText[len(spanText)-1] == '\r'):
			trim = 1
		}
		child.span.End -= trim
		if child.Span().Len() > 0 {
			codeSpanNode.children = append(codeSpanNode.children, child)
		}
		if trim > 0 {
			codeSpanNode.children = append(codeSpanNode.children, &Inline{
				kind: IndentKind,
				span: Span{
					Start: child.Span().End,
					End:   child.Span().End + trim,
				},
				indent: 1,
			})
		}
	}

	nodeCount := nodeIndexForPosition(state.unparsed[state.unparsedPos:], cs.content.End)
	if nodeCount == 0 {
		addSpan(&Inline{
			kind: TextKind,
			span: cs.content,
		})
	} else {
		addSpan(&Inline{
			kind: TextKind,
			span: Span{
				Start: cs.content.Start,
				End:   state.unparsed[state.unparsedPos].Span().End,
			},
		})
		for i := 0; i < nodeCount-1; i++ {
			state.unparsedPos++
			if state.unparsed[state.unparsedPos].Kind() == UnparsedKind {
				addSpan(&Inline{
					kind: TextKind,
					span: state.unparsed[state.unparsedPos].Span(),
				})
			}
		}
		state.unparsedPos++
		addSpan(&Inline{
			kind: TextKind,
			span: Span{
				Start: state.unparsed[state.unparsedPos].Span().Start,
				End:   cs.content.End,
			},
		})
	}

	codeSpanNode.children = p.stripCodeSpanSpace(state, codeSpanNode.children)
	state.addToRoot(codeSpanNode)
}

func (p *InlineParser) stripCodeSpanSpace(state *inlineState, slice []*Inline) []*Inline {
	foundNonSpace := false
	for _, inline := range slice {
		if inline.Kind() != IndentKind && !isOnlySpaces(spanSlice(state.source, inline.Span())) {
			foundNonSpace = true
			break
		}
	}
	if !foundNonSpace {
		return slice
	}

	first, last := slice[0], slice[len(slice)-1]
	if !(first.Kind() == IndentKind || state.source[first.Span().Start] == ' ') ||
		!(last.Kind() == IndentKind || state.source[last.Span().End-1] == ' ') {
		return slice
	}

	if first.Kind() == IndentKind {
		first.indent--
		if first.indent == 0 {
			delete(state.parentMap, first)
			slice = deleteInlineNodes(slice, 0, 1)
		}
	} else {
		first.span.Start++
		if first.Span().Len() == 0 {
			delete(state.parentMap, first)
			slice = deleteInlineNodes(slice, 0, 1)
		}
	}

	if last.Kind() == IndentKind {
		last.indent--
		if last.indent == 0 {
			delete(state.parentMap, last)
			slice = deleteInlineNodes(slice, len(slice)-1, len(slice))
		}
	} else {
		last.span.End--
		if last.Span().Len() == 0 {
			delete(state.parentMap, last)
			slice = deleteInlineNodes(slice, len(slice)-1, len(slice))
		}
	}

	return slice
}

func isOnlySpaces(line []byte) bool {
	for _, c := range line {
		if c != ' ' {
			return false
		}
	}
	return true
}

func parseAutolink(text []byte) (end int) {
	const minSchemeChars = 2
	const maxSchemeChars = 32

	if len(text) < len("<:>")+minSchemeChars || text[0] != '<' {
		return -1
	}
	if emailEnd := parseEmail(text[1:]); emailEnd >= 0 && 1+emailEnd < len(text) && text[1+emailEnd] == '>' {
		return 2 + emailEnd
	}

	// Scheme.
	if !isASCIILetter(text[1]) {
		return -1
	}
	end = 2
	for end < len(text) && (isASCIILetter(text[end]) || isASCIIDigit(text[end]) || strings.IndexByte("+.-", text[end]) >= 0) {
		end++
	}
	if end < 1+minSchemeChars || end > 1+maxSchemeChars {
		return -1
	}
	if end >= len(text) || text[end] != ':' {
		return -1
	}
	end++

	// URI.
	for ; end < len(text); end++ {
		switch {
		case text[end] == '>':
			return end + 1
		case isASCIIControl(text[end]) || text[end] == ' ' || text[end] == '<':
			return -1
		}
	}
	return -1
}

// IsEmailAddress reports whether the string is a CommonMark [email address].
//
// [email address]: https://spec.commonmark.org/0.30/#email-address
func IsEmailAddress(s string) bool {
	return parseEmail([]byte(s)) == len(s)
}

// parseEmail parses an [email address].
//
// [email address]: https://spec.commonmark.org/0.30/#email-address
func parseEmail(text []byte) (end int) {
	for end < len(text) && (isASCIILetter(text[end]) || isASCIIDigit(text[end]) || strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", text[end]) >= 0) {
		end++
	}
	if end == 0 {
		return -1
	}
	if end >= len(text) || text[end] != '@' {
		return -1
	}
	end++

	// Domain name.
	firstLabelLength := parseDomainLabel(text[end:])
	if firstLabelLength < 0 {
		return -1
	}
	end += firstLabelLength
	for end < len(text) && text[end] == '.' {
		end++
		n := parseDomainLabel(text[end:])
		if n < 0 {
			// Dot must be followed by label.
			return -1
		}
		end += n
	}
	return end
}

func parseDomainLabel(text []byte) (end int) {
	if end >= len(text) || !(isASCIILetter(text[0]) || isASCIIDigit(text[0])) {
		return -1
	}
	end++
	for end < 63 && end < len(text) && (isASCIILetter(text[end]) || isASCIIDigit(text[end]) || text[end] == '-') {
		end++
	}
	if text[end-1] == '-' {
		return -1
	}
	if end < len(text) && (isASCIILetter(text[end]) || isASCIIDigit(text[end]) || text[end] == '-') {
		// Label too long.
		return -1
	}
	return end
}

// parseInfoString builds a [InfoStringKind] inline span from the given text,
// handling backslash escapes and entity escapes.
// It assumes that the caller has stripped and leading and trailing whitespace.
func parseInfoString(source []byte, span Span) *Inline {
	plainStart := span.Start
	node := &Inline{
		kind: InfoStringKind,
		span: span,
	}
	for i := span.Start; i < span.End; {
		switch source[i] {
		case '\\':
			if i+1 >= span.End || !isASCIIPunctuation(source[i+1]) {
				i++
				continue
			}
			if plainStart < i {
				node.children = append(node.children, &Inline{
					kind: TextKind,
					span: Span{
						Start: plainStart,
						End:   i,
					},
				})
			}
			node.children = append(node.children, &Inline{
				kind: TextKind,
				span: Span{
					Start: i + 1,
					End:   i + 2,
				},
			})
			i += 2
			plainStart = i
		case '&':
			end := parseCharacterEscape(source[i:span.End])
			if end < 0 {
				i++
				continue
			}
			if plainStart < i {
				node.children = append(node.children, &Inline{
					kind: TextKind,
					span: Span{
						Start: plainStart,
						End:   i,
					},
				})
			}
			node.children = append(node.children, &Inline{
				kind: CharacterReferenceKind,
				span: Span{
					Start: i,
					End:   i + end,
				},
			})
			i += end
			plainStart = i
		default:
			i++
		}
	}
	if plainStart < span.End {
		node.children = append(node.children, &Inline{
			kind: TextKind,
			span: Span{
				Start: plainStart,
				End:   span.End,
			},
		})
	}
	return node
}

func (state *inlineState) addToRoot(newNode *Inline) {
	if newNode.Span().Len() == 0 {
		// Only add nodes that consume at least one source byte.
		return
	}
	state.parentMap[newNode] = state.root
	state.root.children = append(state.root.children, newNode)
}

// wrap inserts a new inline that wraps the nodes between two nodes, exclusive.
// If endNode is nil, then it will wrap all the subsequent siblings of startNode.
func (state *inlineState) wrap(kind InlineKind, startNode, endNode *Inline) *Inline {
	parent := state.parentMap[startNode]
	newNode := &Inline{
		kind: kind,
		span: Span{
			Start: startNode.Span().End,
			End:   parent.Span().End,
		},
	}
	if endNode != nil {
		newNode.span.End = endNode.Span().Start
	}
	state.parentMap[newNode] = parent
	startIndex := 1
	for ; startIndex < len(parent.children); startIndex++ {
		if parent.children[startIndex-1] == startNode {
			break
		}
	}
	if len(parent.children) == 0 || parent.children[startIndex-1] != startNode {
		panic("could not find startNode")
	}
	endIndex := startIndex
	for ; endIndex < len(parent.children); endIndex++ {
		if parent.children[endIndex] == endNode {
			break
		}
	}
	newNode.children = append(newNode.children, parent.children[startIndex:endIndex]...)

	if startIndex == endIndex {
		parent.children = append(parent.children, nil)
		copy(parent.children[endIndex+1:], parent.children[endIndex:])
		parent.children[startIndex] = newNode
	} else {
		parent.children = deleteInlineNodes(parent.children, startIndex+1, endIndex)
	}
	parent.children[startIndex] = newNode

	for _, c := range newNode.children {
		state.parentMap[c] = newNode
	}

	return newNode
}

func (state *inlineState) remove(node *Inline) {
	n := 0
	parent := state.parentMap[node]
	for _, c := range parent.children {
		if c != node {
			parent.children[n] = c
			n++
		}
	}
	parent.children = deleteInlineNodes(parent.children, n, len(parent.children))
	delete(state.parentMap, node)
}

func deleteInlineNodes(slice []*Inline, i, j int) []*Inline {
	copy(slice[i:], slice[j:])
	newEnd := len(slice) - (j - i)
	clear := slice[newEnd:]
	for ci := range clear {
		clear[ci] = nil
	}
	return slice[:newEnd]
}

type delimiterStackElement struct {
	typ   inlineDelimiter
	flags uint8
	n     int
	node  *Inline
}

const openersBottomCount = 9

func (elem delimiterStackElement) openersBottomIndex() int {
	switch elem.typ {
	case inlineDelimiterStar:
		if elem.flags&openerFlag == 0 {
			return elem.n % 3
		} else {
			return 3 + elem.n%3
		}
	case inlineDelimiterUnderscore:
		return 6
	case inlineDelimiterLink:
		return 7
	case inlineDelimiterImage:
		return 8
	default:
		panic("unreachable")
	}
}

func isEmphasisDelimiterMatch(open, close delimiterStackElement) bool {
	return (open.typ == inlineDelimiterStar || open.typ == inlineDelimiterUnderscore) &&
		open.typ == close.typ &&
		open.flags&openerFlag != 0 &&
		close.flags&closerFlag != 0 &&
		// Rule 9 & 10 of https://spec.commonmark.org/0.30/#emphasis-and-strong-emphasis
		(open.flags&closerFlag == 0 && close.flags&openerFlag == 0 ||
			(open.n+close.n)%3 != 0 ||
			open.n%3 == 0 && close.n%3 == 0)
}

func deleteDelimiterStack(stack []delimiterStackElement, i, j int) []delimiterStackElement {
	copy(stack[i:], stack[j:])
	newEnd := len(stack) - (j - i)
	clear := stack[newEnd:]
	for ci := range clear {
		clear[ci] = delimiterStackElement{}
	}
	return stack[:newEnd]
}

const (
	activeFlag = 1 << iota
	openerFlag
	closerFlag
)

type inlineDelimiter int8

const (
	inlineDelimiterStar inlineDelimiter = 1 + iota
	inlineDelimiterUnderscore
	inlineDelimiterLink
	inlineDelimiterImage
)

func (d inlineDelimiter) String() string {
	switch d {
	case inlineDelimiterStar:
		return "*"
	case inlineDelimiterUnderscore:
		return "_"
	case inlineDelimiterLink:
		return "["
	case inlineDelimiterImage:
		return "!["
	default:
		return fmt.Sprintf("inlineDelimiter(%d)", int8(d))
	}
}

// parseHardLineBreakSpace checks for a space-based [hard line break].
//
// [hard line break]: https://spec.commonmark.org/0.30/#hard-line-break
func parseHardLineBreakSpace(remaining []byte) (end int, isHardLineBreak bool) {
	const numSpaces = 2
	for ; end < len(remaining) && end < numSpaces; end++ {
		if remaining[end] != ' ' {
			return end, false
		}
	}
	if end < numSpaces {
		return end, false
	}

	for ; end < len(remaining); end++ {
		if c := remaining[end]; c != ' ' && c != '\n' && c != '\r' {
			return end, false
		}
	}
	return end, true
}

// An inlineByteReader transforms inline nodes into a text stream.
type inlineByteReader struct {
	source     []byte
	spans      []*Inline
	pos        int
	virtualPos int // indent or null replacement
	prevPos    int
}

func newInlineByteReader(source []byte, spans []*Inline, pos int) *inlineByteReader {
	return &inlineByteReader{
		source:  source,
		spans:   spans,
		pos:     pos,
		prevPos: -1,
	}
}

// current returns the byte at the reader's position
// or zero if the reader has reached the end of input.
func (r *inlineByteReader) current() byte {
	if r.pos >= len(r.source) {
		return 0
	}
	if r.currentNode().Kind() == IndentKind {
		return ' '
	}
	if r.source[r.pos] == 0 {
		return nullReplacementString[r.virtualPos]
	}
	return r.source[r.pos]
}

func (r *inlineByteReader) currentNode() *Inline {
	spanIndex := nodeIndexForPosition(r.spans, r.pos)
	if spanIndex < 0 {
		r.spans = nil
		return nil
	}
	r.spans = r.spans[spanIndex:]
	return r.spans[0]
}

func (r *inlineByteReader) remainingNodeBytes() []byte {
	node := r.currentNode()
	if node == nil {
		return nil
	}
	return r.source[r.pos:node.Span().End]
}

func (r *inlineByteReader) next() bool {
	node := r.currentNode()
	if node == nil {
		return false
	}

	// Advance within node if possible.
	if node.Kind() == IndentKind && r.virtualPos < node.IndentWidth() {
		r.prevPos = r.pos
		r.virtualPos++
		return true
	}
	if node.Kind() != IndentKind && r.pos+1 < node.Span().End {
		if r.source[r.pos] == 0 && r.source[r.pos+1] == 0 {
			r.virtualPos = (r.virtualPos + 1) % len(nullReplacementString)
		}
		r.prevPos = r.pos
		r.pos++
		return true
	}

	// Reached end of node. Advance to next inline node.
	r.spans = r.spans[1:]
	for ; len(r.spans) > 0; r.spans = r.spans[1:] {
		if k := r.spans[0].Kind(); k == UnparsedKind || k == TextKind || k == IndentKind {
			r.prevPos = r.pos
			r.pos = r.spans[0].Span().Start
			r.virtualPos = computeNullVirtualPosition(r.source, r.pos)
			return true
		}
	}

	// No next line. Advance position to end of span.
	r.prevPos = r.pos
	r.pos++
	r.spans = nil
	return false
}

func computeNullVirtualPosition(source []byte, pos int) int {
	if pos >= len(source) || source[pos] != 0 {
		return 0
	}
	start := pos
	for start > 0 && source[start-1] == 0 {
		start--
	}
	return (pos - start) % len(nullReplacementString)
}

func (r *inlineByteReader) jumped() bool {
	return r.prevPos >= 0 && r.pos-r.prevPos > 1
}

// nodeIndexForPosition returns the index
// of the first inline node in the slice
// that contains the given position,
// or -1 if no such node exists.
// It assumes that the starts of the inline nodes
// are monotonically increasing.
func nodeIndexForPosition(spans []*Inline, pos int) int {
	search := Span{Start: pos, End: pos + 1}
	for i, inline := range spans {
		inlineSpan := inline.Span()
		if inlineSpan.Start > pos {
			return -1
		}
		if inline.Span().Intersect(search).Len() > 0 {
			return i
		}
	}
	return -1
}

func hasUnparsed(b *Block) bool {
	if b == nil {
		return false
	}
	for _, c := range b.inlineChildren {
		if c.Kind() == UnparsedKind {
			return true
		}
	}
	return false
}
