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
	"bytes"
	"math"
	"unsafe"
)

// RootBlock represents a "top-level" block,
// that is, a block whose parent is the document.
// Root blocks store their Markdown source
// and document position information.
// All other position information in the tree
// is relative to the beginning of the root block.
type RootBlock struct {
	Source      []byte
	StartLine   int
	StartOffset int64
	Block
}

// A Block is a structural element in a Markdown document.
type Block struct {
	kind     BlockKind
	start    int
	end      int
	children []Node

	listDelim      byte // set for [ListKind] and [ListItemKind]
	listItemIndent int  // set for [ListItemKind]
	listLoose      bool // set for [ListKind]
	lastLineBlank  bool
}

func (b *Block) Kind() BlockKind {
	if b == nil {
		return 0
	}
	return b.kind
}

func (b *Block) Start() int {
	if b == nil {
		return -1
	}
	return b.start
}

func (b *Block) End() int {
	if b == nil {
		return -1
	}
	return b.end
}

func (b *Block) Children() []Node {
	if b == nil {
		return nil
	}
	return b.children
}

func (b *Block) HeadingLevel(source []byte) int {
	switch b.Kind() {
	case ATXHeadingKind:
		span := source[b.Start():b.End()]
		for i := 0; i < len(span) && i < 6; i++ {
			if span[i] != '#' {
				return i
			}
		}
		return 6
	default:
		return 0
	}
}

// IsOrderedList reports whether the block is
// an ordered list or an ordered list item.
func (b *Block) IsOrderedList() bool {
	return b != nil && (b.listDelim == '.' || b.listDelim == ')')
}

// IsTightList reports whether the block is
// an tight list or a tight list item.
func (b *Block) IsTightList() bool {
	return b != nil && !b.listLoose
}

// ListItemNumber returns the number of a [ListItemKind] block
// or -1 if the block does not represent an ordered list item.
func (b *Block) ListItemNumber(source []byte) int {
	if !b.IsOrderedList() || b.kind != ListItemKind {
		return -1
	}
	marker := b.firstChild().Inline()
	if marker.Kind() != ListMarkerKind {
		return -1
	}
	parsed := parseListMarker(source[marker.Start():marker.End()])
	if parsed.end < 0 {
		return -1
	}
	return parsed.n
}

func (b *Block) AsNode() Node {
	if b == nil {
		return Node{}
	}
	return Node{
		typ: nodeTypeBlock,
		ptr: unsafe.Pointer(b),
	}
}

func (b *Block) firstChild() Node {
	children := b.Children()
	if len(children) == 0 {
		return Node{}
	}
	return children[0]
}

func (b *Block) lastChild() Node {
	children := b.Children()
	if len(children) == 0 {
		return Node{}
	}
	return children[len(children)-1]
}

func (b *Block) isOpen() bool {
	return b != nil && b.end < 0
}

// close closes b and any open descendents.
// It assumes that only the last child can be open.
// Calling close on a nil block no-ops.
func (b *Block) close(source []byte, end int) {
	for ; b.isOpen(); b = b.lastChild().Block() {
		b.end = end
		if f := blocks[b.kind].onClose; f != nil {
			f(source, b)
		}
	}
}

type BlockKind uint16

const (
	ParagraphKind BlockKind = 1 + iota
	ThematicBreakKind
	ATXHeadingKind
	SetextHeadingKind
	IndentedCodeBlockKind
	FencedCodeBlockKind
	HTMLBlockKind
	LinkReferenceDefinitionKind
	BlockQuoteKind
	ListItemKind
	ListKind

	documentKind
)

// blockParser is a cursor on a line of text,
// used while splitting a document into blocks.
type blockParser struct {
	root      *RootBlock
	container *Block // nil represents the document

	lineStart    int // number of bytes from beginning of root block to start of line
	line         []byte
	i            int  // byte position within line
	col          int  // 0-based column position within line
	tabRemaining int8 // number of columns left within current tab character

	state          int8
	listItemIndent int // indentation of current list item being true
}

// Block parser states.
const (
	// stateOpening is the initial state used in [openNewBlocks].
	stateOpening = iota
	// stateOpenMatched is a state used in [openNewBlocks].
	// It is entered from [stateOpening] after the parser has moved its position
	// or the AST has been modified.
	stateOpenMatched
	// stateLineConsumed is a terminal state used in [openNewBlocks].
	// It is entered from [stateOpening]
	// after [*blockParser.AdvanceToEOL] has been called.
	stateLineConsumed
	// stateDescending is a terminal state used in [descendOpenBlocks].
	// No modification of the AST is permitted in this state.
	stateDescending
)

func newBlockParser(root *RootBlock, line []byte) *blockParser {
	p := &blockParser{
		root:  root,
		state: stateOpening,
	}
	p.reset(0, line)
	return p
}

func (p *blockParser) reset(lineStart int, newSource []byte) {
	p.lineStart = lineStart
	p.root.Source = newSource
	p.line = newSource[lineStart:]
	p.i = 0
	p.col = 0
	p.updateTabRemaining()
	p.listItemIndent = math.MaxInt
}

// BytesAfterIndent returns the bytes
// after any indentation immediately following the cursor.
func (p *blockParser) BytesAfterIndent() []byte {
	return bytes.TrimLeft(p.line[p.i:], " \t")
}

// IsRestBlank reports whether the rest of the line is blank.
func (p *blockParser) IsRestBlank() bool {
	return isBlankLine(p.line[p.i:])
}

// Advance advances the parser by n bytes.
// It panics if n is greater than the number of bytes remaining in the line.
func (p *blockParser) Advance(n int) {
	if n < 0 {
		panic("negative length")
	}
	if n == 0 {
		return
	}
	if p.state == stateOpening {
		p.state = stateOpenMatched
	}
	newIndex := p.i + n
	if newIndex > len(p.line) {
		panic("index out of bounds")
	}
	if p.i < len(p.line) && p.line[p.i] == '\t' {
		p.col += int(p.tabRemaining) + columnWidth(p.col, p.line[p.i+1:newIndex])
	} else {
		p.col += columnWidth(p.col, p.line[p.i:newIndex])
	}
	p.i = newIndex
	p.updateTabRemaining()
}

func (p *blockParser) updateTabRemaining() {
	if p.i < len(p.line) && p.line[p.i] == '\t' {
		p.tabRemaining = int8(columnWidth(p.col, p.line[p.i:p.i+1]))
	} else {
		p.tabRemaining = 0
	}
}

// ConsumeLine advances the cursor past the end of the line.
// This will skip adding text to the block when called during a block start.
func (p *blockParser) ConsumeLine() {
	p.Advance(len(p.line) - p.i)
	if p.state == stateOpening || p.state == stateOpenMatched {
		p.state = stateLineConsumed
	}
}

// Indent returns the number of columns of whitespace
// present after the cursor's position.
func (p *blockParser) Indent() int {
	if p.i >= len(p.line) {
		return 0
	}
	var firstCharWidth int
	switch p.line[p.i] {
	case ' ':
		firstCharWidth = 1
	case '\t':
		firstCharWidth = int(p.tabRemaining)
	default:
		return 0
	}
	rest := p.line[p.i+1:]
	return firstCharWidth + columnWidth(p.col+firstCharWidth, rest[:indentLength(rest)])
}

// ConsumeIndent advances the parser by n columns of whitespace.
// It panics if n is greater than bp.Indent().
func (p *blockParser) ConsumeIndent(n int) {
	for n > 0 {
		if p.state == stateOpening {
			p.state = stateOpenMatched
		}
		switch {
		case p.i < len(p.line) && p.line[p.i] == ' ':
			n--
			p.col++
		case p.i < len(p.line) && p.line[p.i] == '\t':
			if n < int(p.tabRemaining) {
				p.col += n
				p.tabRemaining -= int8(n)
				return
			}
			p.col += int(p.tabRemaining)
			n -= int(p.tabRemaining)
		default:
			panic("consumed past end of indent")
		}

		p.i++
		p.updateTabRemaining()
	}
}

// ContainerKind returns the kind of the current block.
// During block start checks, this will be the parent of block being considered.
// During [blockRule] matches, this will be the same as the rule's kind.
func (p *blockParser) ContainerKind() BlockKind {
	if p.container == nil {
		return documentKind
	}
	return p.container.kind
}

// TipKind returns the kind of the deepest open block.
func (p *blockParser) TipKind() BlockKind {
	tip := findTip(&p.root.Block)
	if tip == nil {
		return documentKind
	}
	return tip.kind
}

func (p *blockParser) ContainerListDelim() byte {
	if p.container == nil {
		return 0
	}
	return p.container.listDelim
}

func (p *blockParser) CurrentItemIndent() int {
	return p.listItemIndent
}

// OpenBlock starts a new block at the current position.
func (p *blockParser) OpenBlock(kind BlockKind) {
	if kind == ListKind || kind == ListItemKind {
		panic("OpenBlock cannot be called with ListKind or ListItemKind")
	}
	p.openBlock(kind)
}

func (p *blockParser) OpenListBlock(kind BlockKind, delim byte) {
	if kind != ListKind && kind != ListItemKind {
		panic("OpenListBlock must be called with ListKind or ListItemKind")
	}
	p.openBlock(kind)
	if p.container != nil {
		p.container.listDelim = delim
	}
}

func (p *blockParser) openBlock(kind BlockKind) {
	switch p.state {
	case stateDescending:
		panic("OpenBlock cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}

	// Move up the tree until we find a block that can handle the new child.
	for {
		if rule := blocks[p.ContainerKind()]; rule.canContain != nil && rule.canContain(kind) {
			break
		}
		p.container.close(p.root.Source, p.lineStart)
		if p.container == nil {
			return
		}
		p.container = findParent(p.root, p.container)
	}

	// Special case: parent is the document.
	if p.container == nil {
		if p.root.kind != 0 {
			// Attempting to open a new root block.
			p.root.close(p.root.Source, p.lineStart)
			return
		}
		p.root.kind = kind
		p.root.start = p.lineStart + p.i
		p.root.end = -1
		p.container = &p.root.Block
		return
	}

	// Normal case: append to the parent's children list.
	p.container.lastChild().Block().close(p.root.Source, p.lineStart)
	newChild := &Block{
		kind:  kind,
		start: p.lineStart + p.i,
		end:   -1,
	}
	p.container.children = append(p.container.children, newChild.AsNode())
	p.container = newChild
}

// SetListItemIndent sets the current list item's indentation.
func (p *blockParser) SetListItemIndent(indent int) {
	switch {
	case p.ContainerKind() == ListItemKind:
		p.container.listItemIndent = indent
	case p.container != nil:
		panic("can't set indent for this block type")
	}
}

// CollectInline adds a new [UnparsedKind] inline to the current block,
// starting at the current position and ending after n bytes.
func (p *blockParser) CollectInline(kind InlineKind, n int) {
	switch p.state {
	case stateDescending:
		panic("CollectInline cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}
	start := p.lineStart + p.i
	p.Advance(n)
	if p.container == nil {
		return
	}
	p.container.children = append(p.container.children, (&Inline{
		kind:  kind,
		start: start,
		end:   p.lineStart + p.i,
	}).AsNode())
}

// EndBlock ends a block at the current position.
func (p *blockParser) EndBlock() {
	switch p.state {
	case stateDescending:
		panic("EndBlock cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}
	if p.container == nil {
		p.root.close(p.root.Source, p.lineStart+p.i)
		return
	}
	p.container.close(p.root.Source, p.lineStart+p.i)
	p.container = findParent(p.root, p.container)
}

// codeBlockIndentLimit is the column width of an indent
// required to start an indented code block.
const codeBlockIndentLimit = 4

const blockQuotePrefix = ">"

var blockStarts = []func(*blockParser){
	// Block quote.
	func(p *blockParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		if !bytes.HasPrefix(p.BytesAfterIndent(), []byte(blockQuotePrefix)) {
			return
		}

		p.ConsumeIndent(indent)
		p.OpenBlock(BlockQuoteKind)
		p.Advance(len(blockQuotePrefix))
		if p.Indent() > 0 {
			p.ConsumeIndent(1)
		}
	},

	// ATX heading.
	func(p *blockParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		h := parseATXHeading(p.BytesAfterIndent())
		if h.level < 1 {
			return
		}

		p.ConsumeIndent(indent)
		p.OpenBlock(ATXHeadingKind)
		p.Advance(h.contentStart)
		p.CollectInline(UnparsedKind, h.contentEnd-h.contentStart)
		p.ConsumeLine()
		p.EndBlock()
	},

	// Thematic break.
	func(p *blockParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		end := parseThematicBreak(p.BytesAfterIndent())
		if end < 0 {
			return
		}

		p.ConsumeIndent(indent)
		p.OpenBlock(ThematicBreakKind)
		p.Advance(end)
		p.EndBlock()
		p.ConsumeLine()
	},

	// List item.
	func(p *blockParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		m := parseListMarker(p.BytesAfterIndent())
		if m.end < 0 || (p.ContainerKind() == ParagraphKind && m.isOrdered() && m.n != 1) {
			return
		}

		p.ConsumeIndent(indent)
		if p.ContainerKind() != ListKind || p.ContainerListDelim() != m.delim {
			p.OpenListBlock(ListKind, m.delim)
		}
		p.OpenListBlock(ListItemKind, m.delim)
		p.CollectInline(ListMarkerKind, m.end)
		padding := p.Indent()
		switch {
		case padding < 1:
			padding = 1
		case padding > 4:
			padding = 1
			p.ConsumeIndent(1)
		default:
			p.ConsumeIndent(padding)
		}
		p.SetListItemIndent(indent + m.end + padding)
	},

	// Indented code block.
	func(p *blockParser) {
		if p.Indent() < codeBlockIndentLimit || p.IsRestBlank() || p.TipKind() == ParagraphKind {
			return
		}
		p.ConsumeIndent(codeBlockIndentLimit)
		p.OpenBlock(IndentedCodeBlockKind)
	},
}

type blockRule struct {
	match        func(*blockParser) bool
	onClose      func(source []byte, block *Block)
	canContain   func(childKind BlockKind) bool
	acceptsLines bool
}

var blocks = map[BlockKind]blockRule{
	documentKind: {
		match:      func(*blockParser) bool { return true },
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	ListKind: {
		match:      func(*blockParser) bool { return true },
		canContain: func(childKind BlockKind) bool { return childKind == ListItemKind },
		onClose: func(source []byte, block *Block) {
			endsWithBlankLine := func(block *Block) bool {
				for block != nil {
					if block.lastLineBlank {
						return true
					}
					if k := block.Kind(); k != ListKind && k != ListItemKind {
						return false
					}
					block = block.lastChild().Block()
				}
				return false
			}

			// Check for a blank line after non-final items.
			items := block.Children()
		determineLoose:
			for i, item := range items {
				if i < len(items)-1 && endsWithBlankLine(item.Block()) {
					block.listLoose = true
					break determineLoose
				}
				subitems := item.Block().Children()
				for j, subitem := range subitems {
					if (i < len(items)-1 || j < len(subitems)-1) &&
						endsWithBlankLine(subitem.Block()) {
						block.listLoose = true
						break determineLoose
					}
				}
			}
			if block.listLoose {
				for _, item := range items {
					item.Block().listLoose = true
				}
			}
		},
	},
	ListItemKind: {
		match: func(p *blockParser) bool {
			indent := p.Indent()
			switch {
			case p.IsRestBlank():
				return true
			case indent >= p.CurrentItemIndent():
				p.ConsumeIndent(p.CurrentItemIndent())
				return true
			default:
				return false
			}
		},
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	BlockQuoteKind: {
		match: func(p *blockParser) bool {
			indent := p.Indent()
			if indent >= codeBlockIndentLimit {
				return false
			}
			if !bytes.HasPrefix(p.BytesAfterIndent(), []byte(blockQuotePrefix)) {
				return false
			}
			p.ConsumeIndent(indent)
			p.Advance(len(blockQuotePrefix))
			if p.Indent() > 0 {
				p.ConsumeIndent(1)
			}
			return true
		},
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	IndentedCodeBlockKind: {
		match: func(p *blockParser) bool {
			indent := p.Indent()
			if p.IsRestBlank() {
				p.ConsumeIndent(indent)
				return true
			}
			if indent < codeBlockIndentLimit {
				return false
			}
			p.ConsumeIndent(codeBlockIndentLimit)
			return true
		},
		onClose: func(source []byte, block *Block) {
			// "Blank lines preceding or following an indented code block are not included in it."
			for i := len(block.children) - 1; i >= 0; i-- {
				child := block.children[i].Inline()
				if child.Kind() != UnparsedKind || !isBlankLine(source[child.Start():child.End()]) {
					break
				}
				block.children[i] = Node{} // free for GC
				block.children = block.children[:i:i]
			}
		},
		acceptsLines: true,
	},
	ParagraphKind: {
		match: func(p *blockParser) bool {
			return !p.IsRestBlank()
		},
		acceptsLines: true,
	},
}

// parseThematicBreak attempts to parse the line as a [thematic break].
// It returns the end of the thematic break characters
// or -1 if the line is not a thematic break.
// parseThematicBreak assumes that the caller has stripped any leading indentation.
//
// [thematic break]: https://spec.commonmark.org/0.30/#thematic-breaks
func parseThematicBreak(line []byte) (end int) {
	n := 0
	var want byte
	for i, b := range line {
		switch b {
		case '-', '_', '*':
			if n == 0 {
				want = b
			} else if b != want {
				return -1
			}
			n++
			end = i + 1
		case ' ', '\t', '\r', '\n':
			// Ignore
		default:
			return -1
		}
	}
	if n < 3 {
		return -1
	}
	return end
}

type atxHeading struct {
	level        int // 1-6
	contentStart int
	contentEnd   int
}

// parseATXHeading attempts to parse the line as an [ATX heading].
// The level is zero if the line is not an ATX heading.
// parseATXHeading assumes that the caller has stripped any leading indentation.
//
// [ATX heading]: https://spec.commonmark.org/0.30/#atx-headings
func parseATXHeading(line []byte) atxHeading {
	var h atxHeading
	for h.level < len(line) && line[h.level] == '#' {
		h.level++
	}
	if h.level == 0 || h.level > 6 {
		return atxHeading{}
	}

	// Consume required whitespace before heading.
	i := h.level
	if i >= len(line) || line[i] == '\n' || line[i] == '\r' {
		h.contentStart = i
		h.contentEnd = i
		return h
	}
	if !(line[i] == ' ' || line[i] == '\t') {
		return atxHeading{}
	}
	i++

	// Advance past leading whitespace.
	for i < len(line) && line[i] == ' ' || line[i] == '\t' {
		i++
	}
	h.contentStart = i

	// Find end of heading line. Skip past trailing spaces.
	h.contentEnd = len(line)
	hitHash := false
scanBack:
	for ; h.contentEnd > h.contentStart; h.contentEnd-- {
		switch line[h.contentEnd-1] {
		case '\r', '\n':
			// Skip past EOL.
		case ' ', '\t':
			if isEndEscaped(line[:h.contentEnd-1]) {
				break scanBack
			}
		case '#':
			hitHash = true
			break scanBack
		default:
			break scanBack
		}
	}
	if !hitHash {
		return h
	}

	// We've encountered one hashmark '#'.
	// Consume all of them, unless they are preceded by a space or tab.
scanTrailingHashes:
	for i := h.contentEnd - 1; ; i-- {
		if i <= h.contentStart {
			h.contentEnd = h.contentStart
			break
		}
		switch line[i] {
		case '#':
			// Keep going.
		case ' ', '\t':
			h.contentEnd = i + 1
			break scanTrailingHashes
		default:
			return h
		}
	}
	// We've hit the end of hashmarks. Trim trailing whitespace.
	for ; h.contentEnd > h.contentStart; h.contentEnd-- {
		if b := line[h.contentEnd-1]; !(b == ' ' || b == '\t') || isEndEscaped(line[:h.contentEnd-1]) {
			break
		}
	}
	return h
}

type listMarker struct {
	delim byte // one of '-', '+', '*', '.', or ')'
	n     int
	end   int // always delimiter position + 1
}

// parseListMarker attempts to parse a [list marker] at the beginning of the line.
// The end is -1 if the line does not begin with a marker.
// parseListMarker assumes that the caller has stripped any leading indentation.
//
// [list marker]: https://spec.commonmark.org/0.30/#list-marker
func parseListMarker(line []byte) listMarker {
	if len(line) == 0 {
		return listMarker{end: -1}
	}
	var n int
	switch c := line[0]; {
	case c == '-' || c == '+' || c == '*':
		if !hasTabOrSpacePrefixOrEOL(line[1:]) {
			return listMarker{end: -1}
		}
		return listMarker{delim: line[0], end: 1}
	case isASCIIDigit(c):
		// Ordered list. Continue.
		n = int(c - '0')
	default:
		return listMarker{end: -1}
	}
	const maxDigits = 9
	for i := 1; i < maxDigits+1 && i < len(line); i++ {
		switch c := line[i]; {
		case isASCIIDigit(c):
			// Continue.
			n *= 10
			n += int(c - '0')
		case c == '.' || c == ')':
			if !hasTabOrSpacePrefixOrEOL(line[i+1:]) {
				return listMarker{end: -1}
			}
			return listMarker{delim: c, n: n, end: i + 1}
		default:
			return listMarker{end: -1}
		}
	}
	return listMarker{end: -1}
}

func (m listMarker) isOrdered() bool {
	return m.delim == '.' || m.delim == ')'
}
