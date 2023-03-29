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
	"strings"
)

// Inline represents CommonMark content elements like text, links, or emphasis.
type Inline struct {
	kind     InlineKind
	start    int
	end      int
	indent   int
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

// Start returns the offset in [RootBlock.Source]
// where the inline node starts,
// or -1 if the node is nil.
func (inline *Inline) Start() int {
	if inline == nil {
		return -1
	}
	return inline.start
}

// End returns the offset in [RootBlock.Source]
// where the inline node ends (exclusive),
// or -1 if the node is nil.
func (inline *Inline) End() int {
	if inline == nil {
		return -1
	}
	return inline.end
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
	case TextKind, ListMarkerKind:
		return string(source[inline.Start():inline.End()])
	case SoftLineBreakKind, HardLineBreakKind:
		return "\n"
	case IndentKind:
		sb := new(strings.Builder)
		for i := 0; i < inline.IndentWidth(); i++ {
			sb.WriteByte(' ')
		}
		return sb.String()
	case InfoStringKind:
		sb := new(strings.Builder)
		sb.Grow(inline.End() - inline.Start())
		for _, child := range inline.Children() {
			switch child.Kind() {
			case TextKind:
				sb.Write(source[child.Start():child.End()])
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// Children returns children of the node.
// Calling Children on nil returns a nil slice.
func (inline *Inline) Children() []*Inline {
	if inline == nil {
		return nil
	}
	return inline.children
}

// InlineKind is an enumeration of values returned by [*Inline.Kind].
type InlineKind uint16

const (
	TextKind InlineKind = 1 + iota
	SoftLineBreakKind
	HardLineBreakKind
	IndentKind
	ListMarkerKind
	InfoStringKind

	CodeSpanKind
	AutolinkKind
	RawHTMLKind

	// UnparsedKind is used for inline text that has not been tokenized.
	UnparsedKind
)

// An InlineParser converts [UnparsedKind] [Inline] nodes
// into inline trees.
type InlineParser struct {
}

// Rewrite replaces any [UnparsedKind] nodes in the given root block
// with parsed versions of the node.
func (p *InlineParser) Rewrite(root *RootBlock) {
	stack := []*Block{&root.Block}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if blocks[curr.Kind()].acceptsLines {
			if hasUnparsed(curr) {
				curr.children = p.parse(nil, root.Source, curr.Kind(), curr.Children())
			}
		} else {
			for i := len(curr.children) - 1; i >= 0; i-- {
				if b := curr.children[i].Block(); b != nil {
					stack = append(stack, b)
				}
			}
		}
	}
}

type inlineState struct {
	dst        []Node
	source     []byte
	spanEnd    int
	isLastSpan bool
	blockKind  BlockKind
	container  *Inline
	stack      []delimiterStackElement
}

func (p *InlineParser) parse(dst []Node, source []byte, containerKind BlockKind, unparsed []Node) []Node {
	state := &inlineState{
		dst:       dst,
		source:    source,
		blockKind: containerKind,
	}
	for i, u := range unparsed {
		switch u.Inline().Kind() {
		case 0:
		case UnparsedKind:
			state.spanEnd = u.Inline().End()
			state.isLastSpan = i == len(unparsed)-1
			plainStart := u.Inline().Start()
			for i := plainStart; i < state.spanEnd; {
				switch source[i] {
				case '*', '_':
					state.add(&Inline{
						kind:  TextKind,
						start: plainStart,
						end:   i,
					})
					i = p.parseEmphasis(state, i)
					plainStart = i
				case '\\':
					if containerKind.IsCode() ||
						state.container.Kind() == CodeSpanKind ||
						state.container.Kind() == AutolinkKind ||
						state.container.Kind() == RawHTMLKind {
						i++
					} else {
						state.add(&Inline{
							kind:  TextKind,
							start: plainStart,
							end:   i,
						})
						i = p.parseBackslash(state, i)
						plainStart = i
					}
				default:
					i++
				}
			}
			state.add(&Inline{
				kind:  TextKind,
				start: plainStart,
				end:   state.spanEnd,
			})
		default:
			state.dst = append(state.dst, u)
		}
	}
	return state.dst
}

func (p *InlineParser) parseBackslash(state *inlineState, start int) (end int) {
	if start+1 >= state.spanEnd || state.source[start+1] == '\n' || state.source[start+1] == '\r' {
		// At end of line.
		newNode := &Inline{
			kind:  HardLineBreakKind,
			start: start,
			end:   start + 1,
		}
		if state.isLastSpan {
			// Hard line breaks not permitted at end of block.
			newNode.kind = TextKind
		}
		state.add(newNode)
		return newNode.end
	}
	if isASCIIPunctuation(state.source[start+1]) {
		start++
		end = start + 1
		state.add(&Inline{
			kind:  TextKind,
			start: start,
			end:   end,
		})
		return end
	}
	end = start + 2
	state.add(&Inline{
		kind:  TextKind,
		start: start,
		end:   end,
	})
	return end
}

func (p *InlineParser) parseEmphasis(state *inlineState, start int) (end int) {
	node := &Inline{
		kind:  TextKind,
		start: start,
		end:   start + 1,
	}
	for node.end < state.spanEnd && state.source[node.end] == state.source[node.start] {
		node.end++
	}

	elem := delimiterStackElement{
		flags: activeFlag,
		n:     node.end - node.start,
		node:  node,
	}
	if state.source[node.start] == '*' {
		elem.typ = inlineDelimiterStar
	} else {
		elem.typ = inlineDelimiterUnderscore
	}
	// TODO(now)
	state.add(node)
	_ = elem

	return node.end
}

// parseInfoString builds a [InfoStringKind] inline span from the given text,
// handling backslash escapes and entity escapes.
// It assumes that the caller has stripped and leading and trailing whitespace.
func parseInfoString(source []byte, start, end int) *Inline {
	plainStart := start
	node := &Inline{
		kind:  InfoStringKind,
		start: start,
		end:   end,
	}
	for i := start; i < end; {
		// TODO(soon): Entity escapes.
		switch source[i] {
		case '\\':
			if i+1 >= end || !isASCIIPunctuation(source[i+1]) {
				i++
				continue
			}
			if plainStart < i {
				node.children = append(node.children, &Inline{
					kind:  TextKind,
					start: plainStart,
					end:   i,
				})
			}
			node.children = append(node.children, &Inline{
				kind:  TextKind,
				start: i + 1,
				end:   i + 2,
			})
			i += 2
			plainStart = i
		default:
			i++
		}
	}
	if plainStart < end {
		node.children = append(node.children, &Inline{
			kind:  TextKind,
			start: plainStart,
			end:   end,
		})
	}
	return node
}

func (state *inlineState) add(newNode *Inline) {
	if state.container == nil {
		state.dst = append(state.dst, newNode.AsNode())
	} else {
		state.container.children = append(state.container.children, newNode)
	}
}

type delimiterStackElement struct {
	typ   inlineDelimiter
	flags uint8
	n     int
	node  *Inline
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

func hasUnparsed(b *Block) bool {
	for _, c := range b.Children() {
		if c.Inline().Kind() == UnparsedKind {
			return true
		}
	}
	return false
}
