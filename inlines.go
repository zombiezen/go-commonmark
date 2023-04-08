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
	case TextKind:
		return string(source[inline.Start():inline.End()])
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
		sb.Grow(inline.End() - inline.Start())
		for i, n := 0, inline.ChildCount(); i < n; i++ {
			switch child := inline.Child(i); child.Kind() {
			case TextKind:
				sb.Write(source[child.Start():child.End()])
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
	if inline.Kind() != LinkKind {
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
	if inline.Kind() != LinkKind {
		return nil
	}
	for i := len(inline.children) - 1; i >= len(inline.children)-2 && i >= 0; i-- {
		if child := inline.children[i]; child.Kind() == LinkTitleKind {
			return child
		}
	}
	return nil
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
	TextKind InlineKind = 1 + iota
	SoftLineBreakKind
	HardLineBreakKind
	IndentKind
	InfoStringKind
	EmphasisKind
	StrongKind
	LinkKind
	ImageKind
	LinkDestinationKind
	LinkTitleKind

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
	blockKind        BlockKind
	stack            []delimiterStackElement
	ignoreNextIndent bool
	parentMap        map[*Inline]*Inline
}

func (p *inlineState) spanEnd() int {
	if len(p.unparsed) == 0 {
		return len(p.source)
	}
	return p.unparsed[0].end
}

func (p *inlineState) isLastSpan() bool {
	return len(p.unparsed) <= 1
}

func (p *InlineParser) parse(source []byte, container *Block) []*Inline {
	dummy := &Inline{
		start: container.start,
		end:   container.end,
	}
	state := &inlineState{
		root:      dummy,
		source:    source,
		blockKind: container.Kind(),
		unparsed:  container.inlineChildren,
		parentMap: make(map[*Inline]*Inline),
	}
	for ; len(state.unparsed) > 0; state.unparsed = state.unparsed[1:] {
		switch state.unparsed[0].Kind() {
		case 0:
			state.ignoreNextIndent = false
		case IndentKind:
			if !state.ignoreNextIndent {
				dummy.children = append(dummy.children, state.unparsed[0])
			}
			state.ignoreNextIndent = false
		case UnparsedKind:
			state.ignoreNextIndent = false
			plainStart := state.unparsed[0].Start()
			for pos := plainStart; len(state.unparsed) > 0 && pos < state.spanEnd(); {
				switch source[pos] {
				case '*', '_':
					state.addToRoot(&Inline{
						kind:  TextKind,
						start: plainStart,
						end:   pos,
					})
					pos = p.parseDelimiterRun(state, pos)
					plainStart = pos
				case '[':
					state.addToRoot(&Inline{
						kind:  TextKind,
						start: plainStart,
						end:   pos,
					})
					node := &Inline{
						kind:  TextKind,
						start: pos,
						end:   pos + 1,
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
						kind:  TextKind,
						start: plainStart,
						end:   pos,
					})
					pos = p.parseEndBracket(state, pos)
					plainStart = pos
				case ' ':
					end, ok := parseHardLineBreakSpace(source[pos:state.spanEnd()])
					if ok && !state.isLastSpan() {
						state.addToRoot(&Inline{
							kind:  TextKind,
							start: plainStart,
							end:   pos,
						})
						state.addToRoot(&Inline{
							kind:  HardLineBreakKind,
							start: pos,
							end:   pos + end,
						})
						// Leading spaces at the beginning of the next line are ignored.
						state.ignoreNextIndent = true
						plainStart = pos + end
					}
					pos += end
				case '`':
					if cs := p.parseCodeSpan(state, pos); cs.end >= 0 {
						state.addToRoot(&Inline{
							kind:  TextKind,
							start: plainStart,
							end:   cs.start,
						})
						p.collectCodeSpan(state, cs)

						pos = cs.end
						plainStart = pos
					} else {
						// Advance past literal backtick string.
						pos = cs.contentStart
					}
				case '\\':
					state.addToRoot(&Inline{
						kind:  TextKind,
						start: plainStart,
						end:   pos,
					})
					pos = p.parseBackslash(state, pos)
					plainStart = pos
				default:
					pos++
				}
			}
			state.addToRoot(&Inline{
				kind:  TextKind,
				start: plainStart,
				end:   state.spanEnd(),
			})
		default:
			state.ignoreNextIndent = false
			dummy.children = append(dummy.children, state.unparsed[0])
		}
	}
	p.processEmphasis(state, 0)
	return dummy.children
}

func (p *InlineParser) parseBackslash(state *inlineState, start int) (end int) {
	if start+1 >= state.spanEnd() || state.source[start+1] == '\n' || state.source[start+1] == '\r' {
		// At end of line.
		newNode := &Inline{
			kind:  HardLineBreakKind,
			start: start,
			end:   start + 1,
		}
		if state.isLastSpan() {
			// Hard line breaks not permitted at end of block.
			newNode.kind = TextKind
		} else {
			// Leading spaces at the beginning of the next line are ignored.
			state.ignoreNextIndent = true
		}
		state.addToRoot(newNode)
		return newNode.end
	}
	if isASCIIPunctuation(state.source[start+1]) {
		start++
		end = start + 1
		state.addToRoot(&Inline{
			kind:  TextKind,
			start: start,
			end:   end,
		})
		return end
	}
	end = start + 2
	state.addToRoot(&Inline{
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
	for node.end < state.spanEnd() && state.source[node.end] == state.source[node.start] {
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

	state.addToRoot(node)
	state.stack = append(state.stack, elem)
	return node.end
}

func (p *InlineParser) parseEndBracket(state *inlineState, start int) (end int) {
	openDelimIndex := p.lookForLinkOrImage(state)
	if openDelimIndex < 0 {
		state.addToRoot(&Inline{
			kind:  TextKind,
			start: start,
			end:   start + 1,
		})
		return start + 1
	}
	kind := LinkKind
	if state.stack[openDelimIndex].typ == inlineDelimiterImage {
		kind = ImageKind
	}
	// TODO(soon): Add more link types.
	switch {
	case start+1 < state.spanEnd() && state.source[start+1] == '(':
		info := p.parseInlineLink(state, start+1)
		if info.start < 0 {
			state.addToRoot(&Inline{
				kind:  TextKind,
				start: start,
				end:   start + 1,
			})
			return start + 1
		}

		linkNode := state.wrap(kind, state.stack[openDelimIndex].node, nil)
		if info.destination.start >= 0 {
			destNode := &Inline{
				kind:  LinkDestinationKind,
				start: info.destination.start,
				end:   info.destination.end,
			}
			p.addLinkAttributeText(state.source, info.lines, destNode, info.destination.textStart, info.destination.textEnd)
			linkNode.children = append(linkNode.children, destNode)
		}
		if info.titleStart >= 0 {
			linkNode.children = append(linkNode.children, &Inline{
				kind:  LinkTitleKind,
				start: info.titleStart,
				end:   info.titleEnd,
			})
		}
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
		return info.end
	default:
		state.stack = deleteDelimiterStack(state.stack, openDelimIndex, openDelimIndex+1)
		state.addToRoot(&Inline{
			kind:  TextKind,
			start: start,
			end:   start + 1,
		})
		return start + 1
	}
}

type inlineLinkInfo struct {
	lines       [2]*Inline
	start       int
	end         int
	destination linkDestination
	titleStart  int
	titleEnd    int
}

func (p *InlineParser) parseInlineLink(state *inlineState, start int) (result inlineLinkInfo) {
	lines := [2]*Inline{state.unparsed[0]}
	for i, u := range state.unparsed[1:] {
		if u.Kind() == UnparsedKind {
			lines[1] = u
			defer func(nextLineIndex int) {
				if result.end > lines[1].Start() {
					state.unparsed = state.unparsed[nextLineIndex:]
				}
			}(i)
			break
		}
	}
	origLines := lines

	// Skip initial opening parenthesis.
	pos := start + 1
	// Skip any leading spaces.
	for {
		if pos >= lines[0].End() {
			if lines[1] == nil {
				return inlineLinkInfo{start: -1, end: -1}
			}
			lines[0], lines[1] = lines[1], nil
			pos = lines[0].start
			continue
		}
		if state.source[pos] != ' ' && state.source[pos] != '\t' {
			break
		}
		pos++
	}

	result = inlineLinkInfo{
		lines:      origLines,
		start:      start,
		end:        -1,
		titleStart: -1,
		titleEnd:   -1,
	}
	result.destination = p.parseLinkDestination(state.source, lines, pos)
	if result.destination.end >= 0 {
		pos = result.destination.end
		if pos >= lines[0].End() {
			lines[0], lines[1] = lines[1], nil
		}
		// TODO(now): Skip spaces.
	}
	// TODO(now): Title
	if pos >= lines[0].End() || state.source[pos] != ')' {
		return inlineLinkInfo{start: -1, end: -1}
	}
	result.end = pos + 1
	return result
}

type linkDestination struct {
	start     int
	end       int
	textStart int
	textEnd   int
}

func (p *InlineParser) parseLinkDestination(source []byte, lines [2]*Inline, start int) linkDestination {
	switch c := source[start]; {
	case c == '<':
		for pos := start + 1; pos < lines[0].End(); {
			switch source[pos] {
			case '\\':
				pos += 2
			case '>':
				return linkDestination{
					start:     start,
					textStart: start + 1,
					textEnd:   pos,
					end:       pos + 1,
				}
			default:
				pos++
			}
		}
		return linkDestination{
			start:     -1,
			end:       -1,
			textStart: -1,
			textEnd:   -1,
		}
	case !isASCIIControl(c) && c != ' ':
		parenCount := 0
		for pos := start; ; {
			if pos >= lines[0].end {
				if lines[1] == nil {
					if start < lines[0].end {
						return linkDestination{
							start:     start,
							textStart: start,
							textEnd:   pos,
							end:       pos,
						}
					} else {
						return linkDestination{
							start:     -1,
							end:       -1,
							textStart: -1,
							textEnd:   -1,
						}
					}
				}
				lines[0], lines[1] = lines[1], nil
				pos = lines[0].start
				continue
			}
			switch c := source[pos]; {
			case isASCIIControl(c) || c == ' ':
				return linkDestination{
					start:     -1,
					end:       -1,
					textStart: -1,
					textEnd:   -1,
				}
			case c == '\\':
				pos += 2
			case c == '(':
				parenCount++
				pos++
			case c == ')':
				parenCount--
				if parenCount < 0 {
					if start < pos {
						return linkDestination{
							start:     start,
							textStart: start,
							textEnd:   pos,
							end:       pos,
						}
					} else {
						return linkDestination{
							start:     -1,
							end:       -1,
							textStart: -1,
							textEnd:   -1,
						}
					}
				}
				pos++
			default:
				pos++
			}
		}
	default:
		return linkDestination{
			start:     -1,
			end:       -1,
			textStart: -1,
			textEnd:   -1,
		}
	}
}

func (p *InlineParser) addLinkAttributeText(source []byte, lines [2]*Inline, parent *Inline, start, end int) {
	plainStart := start
	for pos := start; pos < end; {
		if pos >= lines[0].End() {
			if lines[1] == nil {
				break
			}
			if plainStart < pos {
				parent.children = append(parent.children, &Inline{
					kind:  TextKind,
					start: plainStart,
					end:   lines[0].End(),
				})
			}
			lines[0], lines[1] = lines[1], nil
			pos = lines[0].start
			plainStart = pos
			continue
		}
		switch source[pos] {
		case '\\':
			if pos+1 >= end || pos+1 >= lines[0].End() || !isASCIIPunctuation(source[pos+1]) {
				pos++
				continue
			}
			if plainStart < pos {
				parent.children = append(parent.children, &Inline{
					kind:  TextKind,
					start: plainStart,
					end:   pos,
				})
			}
			plainStart = pos + 1
			pos += 2
		default:
			pos++
		}
	}
	if plainStart < end {
		parent.children = append(parent.children, &Inline{
			kind:  TextKind,
			start: plainStart,
			end:   end,
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
			state.stack = deleteDelimiterStack(state.stack, openerIndex+1, currentPosition)
			currentPosition = openerIndex + 1

			// If either the opening or the closing text nodes became empty,
			// remove them from the tree.
			if opener.start == opener.end {
				state.remove(opener)
				state.stack = deleteDelimiterStack(state.stack, openerIndex, openerIndex+1)
				currentPosition--
			}
			if closer.start == closer.end {
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
	nodeCount    int
	start        int
	contentStart int
	contentEnd   int
	end          int
}

func (p *InlineParser) parseCodeSpan(state *inlineState, start int) codeSpan {
	result := codeSpan{
		start:        start,
		contentStart: start,
		contentEnd:   -1,
		end:          -1,
	}
	backtickLength := 0
	for result.contentStart < state.spanEnd() && state.source[result.contentStart] == '`' {
		backtickLength++
		result.contentStart++
	}

	result.contentEnd = result.contentStart
	for {
		if result.contentEnd >= state.unparsed[result.nodeCount].End() {
			for {
				result.nodeCount++
				if result.nodeCount >= len(state.unparsed) {
					// Hit end of input before encountering end of code span.
					result.contentEnd = -1
					return result
				}
				if state.unparsed[result.nodeCount].Kind() == UnparsedKind {
					break
				}
			}
			result.contentEnd = state.unparsed[result.nodeCount].Start()
		}

		if state.source[result.contentEnd] != '`' {
			result.contentEnd++
			continue
		}
		currentRunLength := 1
		peekPos := result.contentEnd + 1
		for peekPos < state.unparsed[result.nodeCount].End() && state.source[peekPos] == '`' {
			currentRunLength++
			peekPos++
		}
		if currentRunLength == backtickLength {
			result.end = peekPos
			return result
		}

		result.contentEnd = peekPos
	}
}

func (p *InlineParser) collectCodeSpan(state *inlineState, cs codeSpan) {
	codeSpanNode := &Inline{
		kind:  CodeSpanKind,
		start: cs.start,
		end:   cs.end,
	}
	addSpan := func(child *Inline) {
		spanText := state.source[child.Start():child.End()]
		trim := 0
		switch {
		case len(spanText) >= 2 && spanText[len(spanText)-2] == '\r' && spanText[len(spanText)-1] == '\n':
			trim = 2
		case len(spanText) >= 1 && (spanText[len(spanText)-1] == '\n' || spanText[len(spanText)-1] == '\r'):
			trim = 1
		}
		child.end -= trim
		if child.start < child.end {
			codeSpanNode.children = append(codeSpanNode.children, child)
		}
		if trim > 0 {
			codeSpanNode.children = append(codeSpanNode.children, &Inline{
				kind:   IndentKind,
				start:  child.end,
				end:    child.end + trim,
				indent: 1,
			})
		}
	}

	if cs.nodeCount == 0 {
		addSpan(&Inline{
			kind:  TextKind,
			start: cs.contentStart,
			end:   cs.contentEnd,
		})
	} else {
		addSpan(&Inline{
			kind:  TextKind,
			start: cs.contentStart,
			end:   state.unparsed[0].End(),
		})
		for i := 0; i < cs.nodeCount-1; i++ {
			state.unparsed = state.unparsed[1:]
			addSpan(&Inline{
				kind:  TextKind,
				start: state.unparsed[0].Start(),
				end:   state.unparsed[0].End(),
			})
		}
		state.unparsed = state.unparsed[1:]
		addSpan(&Inline{
			kind:  TextKind,
			start: state.unparsed[0].Start(),
			end:   cs.contentEnd,
		})
	}

	codeSpanNode.children = p.stripCodeSpanSpace(state, codeSpanNode.children)
	state.addToRoot(codeSpanNode)
}

func (p *InlineParser) stripCodeSpanSpace(state *inlineState, slice []*Inline) []*Inline {
	foundNonSpace := false
	for _, inline := range slice {
		if inline.Kind() != IndentKind && !isOnlySpaces(state.source[inline.Start():inline.End()]) {
			foundNonSpace = true
			break
		}
	}
	if !foundNonSpace {
		return slice
	}

	first, last := slice[0], slice[len(slice)-1]
	if !(first.Kind() == IndentKind || state.source[first.Start()] == ' ') ||
		!(last.Kind() == IndentKind || state.source[last.End()-1] == ' ') {
		return slice
	}

	if first.Kind() == IndentKind {
		first.indent--
		if first.indent == 0 {
			delete(state.parentMap, first)
			slice = deleteInlineNodes(slice, 0, 1)
		}
	} else {
		first.start++
		if first.start == first.end {
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
		last.end--
		if last.start == last.end {
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

func (state *inlineState) addToRoot(newNode *Inline) {
	if newNode.start == newNode.end {
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
		kind:  kind,
		start: startNode.end,
		end:   parent.end,
	}
	if endNode != nil {
		newNode.end = endNode.start
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
