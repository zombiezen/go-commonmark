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
	"unicode/utf8"
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
	EmphasisKind
	StrongKind

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
				curr.children = p.parse(nil, root.Source, curr, curr.Children())
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
	container  Node
	stack      []delimiterStackElement
	parentMap  map[*Inline]Node
}

func (p *InlineParser) parse(dst []Node, source []byte, container *Block, unparsed []Node) []Node {
	state := &inlineState{
		dst:       dst,
		source:    source,
		blockKind: container.Kind(),
		container: container.AsNode(),
		parentMap: make(map[*Inline]Node),
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
					i = p.parseDelimiterRun(state, i)
					plainStart = i
				case '\\':
					if k := state.container.Inline().Kind(); k == CodeSpanKind || k == AutolinkKind || k == RawHTMLKind {
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
	p.processEmphasis(state, 0)
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

func (p *InlineParser) parseDelimiterRun(state *inlineState, start int) (end int) {
	node := &Inline{
		kind:  TextKind,
		start: start,
		end:   start + 1,
	}
	for node.end < state.spanEnd && state.source[node.end] == state.source[node.start] {
		node.end++
	}

	elem := delimiterStackElement{
		flags: activeFlag | emphasisFlags(state.source, node.start, node.end),
		n:     node.end - node.start,
		node:  node,
	}
	if state.source[node.start] == '*' {
		elem.typ = inlineDelimiterStar
	} else {
		elem.typ = inlineDelimiterUnderscore
	}

	state.add(node)
	state.stack = append(state.stack, elem)
	return node.end
}

// emphasisFlags determines whether the given [delimiter run]
// [can open emphasis] and/or [can close emphasis].
//
// [delimiter run]: https://spec.commonmark.org/0.30/#delimiter-run
// [can open emphasis]: https://spec.commonmark.org/0.30/#can-open-emphasis
// [can close emphasis]: https://spec.commonmark.org/0.30/#can-close-emphasis
func emphasisFlags(source []byte, start, end int) uint8 {
	var flags uint8
	prevChar := ' '
	if start > 0 {
		prevChar, _ = utf8.DecodeLastRune(source[:start])
	}
	nextChar := ' '
	if end < len(source) {
		nextChar, _ = utf8.DecodeRune(source[end:])
	}
	leftFlanking := !isUnicodeWhitespace(nextChar) &&
		(!isUnicodePunctuation(nextChar) || isUnicodeWhitespace(prevChar) || isUnicodePunctuation(prevChar))
	rightFlanking := !isUnicodeWhitespace(prevChar) &&
		(!isUnicodePunctuation(prevChar) || isUnicodeWhitespace(nextChar) || isUnicodePunctuation(nextChar))
	if leftFlanking && (source[start] == '*' || !rightFlanking || isUnicodePunctuation(prevChar)) {
		flags |= openerFlag
	}
	if rightFlanking && (source[start] == '*' || !leftFlanking || isUnicodePunctuation(nextChar)) {
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
			strong := opener.end-opener.start >= 2 && closer.end-closer.start >= 2
			if strong {
				opener.end -= 2
				closer.start += 2
				state.wrap(StrongKind, opener, closer)
			} else {
				opener.end--
				closer.start++
				state.wrap(EmphasisKind, opener, closer)
			}

			// Remove any delimiters between the opener and closer from the delimiter stack.
			state.stack = removeFromDelimiterStack(state.stack, openerIndex+1, currentPosition)
			currentPosition = openerIndex + 1

			// If either the opening or the closing text nodes became empty,
			// remove them from the tree.
			if opener.start == opener.end {
				state.remove(opener)
				state.stack = removeFromDelimiterStack(state.stack, openerIndex, openerIndex+1)
				currentPosition--
			}
			if closer.start == closer.end {
				state.remove(closer)
				state.stack = removeFromDelimiterStack(state.stack, currentPosition, currentPosition+1)
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
	state.stack = removeFromDelimiterStack(state.stack, stackBottom, len(state.stack))
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
	state.parentMap[newNode] = state.container
	if inline := state.container.Inline(); inline != nil {
		inline.children = append(inline.children, newNode)
	} else {
		state.dst = append(state.dst, newNode.AsNode())
	}
}

// wrap inserts a new inline that wraps the nodes between two nodes, exclusive.
func (state *inlineState) wrap(kind InlineKind, startNode, endNode *Inline) {
	parent := state.parentMap[startNode]
	newNode := &Inline{
		kind:  kind,
		start: startNode.end,
		end:   endNode.start,
	}
	state.parentMap[newNode] = parent
	if inlineParent := parent.Inline(); inlineParent != nil {
		startIndex := 1
		for ; startIndex < len(inlineParent.children); startIndex++ {
			if inlineParent.children[startIndex-1] == startNode {
				break
			}
		}
		if len(inlineParent.children) == 0 || inlineParent.children[startIndex-1] != startNode {
			panic("could not find startNode")
		}

		endIndex := startIndex
		for ; endIndex < len(inlineParent.children); endIndex++ {
			if inlineParent.children[endIndex] == endNode {
				break
			}
		}

		newNode.children = append(newNode.children, inlineParent.children[startIndex:endIndex]...)

		if startIndex == endIndex {
			inlineParent.children = append(inlineParent.children, nil)
			copy(inlineParent.children[endIndex+1:], inlineParent.children[endIndex:])
			inlineParent.children[startIndex] = newNode
		} else {
			copy(inlineParent.children[startIndex+1:], inlineParent.children[endIndex:])
			inlineParent.children = inlineParent.children[:len(inlineParent.children)-(endIndex-startIndex-1)]
		}
		inlineParent.children[startIndex] = newNode
	} else {
		startIndex := 1
		for ; startIndex < len(state.dst); startIndex++ {
			if state.dst[startIndex-1].Inline() == startNode {
				break
			}
		}
		if len(state.dst) == 0 || state.dst[startIndex-1].Inline() != startNode {
			panic("could not find startNode")
		}

		endIndex := startIndex
		for ; endIndex < len(state.dst); endIndex++ {
			if state.dst[endIndex].Inline() == endNode {
				break
			}
		}

		for _, c := range state.dst[startIndex:endIndex] {
			if c := c.Inline(); c != nil {
				newNode.children = append(newNode.children, c)
			}
		}

		if startIndex == endIndex {
			state.dst = append(state.dst, Node{})
			copy(state.dst[endIndex+1:], state.dst[endIndex:])
			state.dst[startIndex] = newNode.AsNode()
		} else {
			copy(state.dst[startIndex+1:], state.dst[endIndex:])
			state.dst = state.dst[:len(state.dst)-(endIndex-startIndex-1)]
		}
		state.dst[startIndex] = newNode.AsNode()
	}

	for _, c := range newNode.children {
		state.parentMap[c] = newNode.AsNode()
	}
}

func (state *inlineState) remove(node *Inline) {
	if inline := state.container.Inline(); inline != nil {
		n := 0
		for _, c := range inline.children {
			if c != node {
				inline.children[n] = c
				n++
			}
		}
		clear := inline.children[n:]
		for i := range clear {
			clear[i] = nil
		}
		inline.children = inline.children[:n]
	} else {
		n := 0
		for _, c := range state.dst {
			if c.Inline() != node {
				state.dst[n] = c
				n++
			}
		}
		clear := state.dst[n:]
		for i := range clear {
			clear[i] = Node{}
		}
		state.dst = state.dst[:n]
	}

	delete(state.parentMap, node)
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

func removeFromDelimiterStack(stack []delimiterStackElement, i, j int) []delimiterStackElement {
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

func hasUnparsed(b *Block) bool {
	for _, c := range b.Children() {
		if c.Inline().Kind() == UnparsedKind {
			return true
		}
	}
	return false
}
