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

//go:generate stringer -type=BlockKind,InlineKind -output=kind_string.go

package commonmark

import (
	"bytes"
	"math"
)

// RootBlock represents a "top-level" block,
// that is, a block whose parent is the document.
// Root blocks store their CommonMark source
// and document position information.
// All other position information in the tree
// is relative to the beginning of the root block.
type RootBlock struct {
	// Source holds the bytes of the block read from the original source.
	// Any NUL bytes will have been replaced with the Unicode Replacement Character.
	Source []byte
	// StartLine is the 1-based line number of the first line of the block.
	StartLine int
	// StartOffset is the byte offset from the beginning of the original source
	// that this block starts at.
	StartOffset int64
	// EndOffset is the byte offset from the beginning of the original source
	// that this block ends at.
	// Unless the original source contained NUL bytes,
	// EndOffset = StartOffset + len(Source).
	EndOffset int64

	Block
}

// A Block is a structural element in a CommonMark document.
type Block struct {
	kind BlockKind
	span Span

	// At most one of blockChildren or inlineChildren can be set.

	blockChildren  []*Block
	inlineChildren []*Inline

	// indent is the block's indentation.
	// For [ListItemKind], it is the number of columns required to continue the block.
	// For [FencedCodeBlockKind], it is the number of columns
	// to strip at the beginning of each line.
	indent int

	// n is a kind-specific datum.
	// For [ATXHeadingKind] and [SetextHeadingKind], it is the level of the heading.
	// For [FencedCodeBlockKind], it is the number of characters used in the starting code fence.
	// For [HTMLBlockKind], it is the index in [htmlBlockConditions]
	// that started this block.
	n int

	// char is a kind-specific datum.
	// For [ListKind] and [ListItemKind], it is the character at the end of the list marker.
	// For [FencedCodeBlockKind], it is the character of the fence.
	char byte

	listLoose     bool // valid for [ListKind] and [ListItemKind]
	lastLineBlank bool
}

// Kind returns the type of block node
// or zero if the node is nil.
func (b *Block) Kind() BlockKind {
	if b == nil {
		return 0
	}
	return b.kind
}

// Span returns the position information relative to the [RootBlock]'s Source field.
func (b *Block) Span() Span {
	if b == nil {
		return NullSpan()
	}
	return b.span
}

// ChildCount returns the number of children the node has.
// Calling ChildCount on nil returns 0.
func (b *Block) ChildCount() int {
	switch {
	case b == nil:
		return 0
	case len(b.blockChildren) > 0:
		return len(b.blockChildren)
	default:
		return len(b.inlineChildren)
	}
}

// Child returns the i'th child of the node.
func (b *Block) Child(i int) Node {
	if len(b.blockChildren) > 0 {
		return b.blockChildren[i].AsNode()
	} else {
		return b.inlineChildren[i].AsNode()
	}
}

// HeadingLevel returns the 1-based level for an [ATXHeadingKind] or [SetextHeadingKind],
// or zero otherwise.
func (b *Block) HeadingLevel() int {
	switch b.Kind() {
	case ATXHeadingKind, SetextHeadingKind:
		return b.n
	default:
		return 0
	}
}

// IsOrderedList reports whether the block is
// an ordered list or an ordered list item.
func (b *Block) IsOrderedList() bool {
	return b != nil && (b.char == '.' || b.char == ')')
}

// IsTightList reports whether the block is
// a tight list or a tight list item.
func (b *Block) IsTightList() bool {
	return b != nil && (b.kind == ListKind || b.kind == ListItemKind) && !b.listLoose
}

// ListItemNumber returns the number of a [ListItemKind] block
// or -1 if the block does not represent an ordered list item.
func (b *Block) ListItemNumber(source []byte) int {
	if !b.IsOrderedList() || b.kind != ListItemKind {
		return -1
	}
	marker := b.firstChild().Block()
	if marker.Kind() != ListMarkerKind {
		return -1
	}
	parsed := parseListMarker(spanSlice(source, marker.Span()))
	if parsed.end < 0 {
		return -1
	}
	return parsed.n
}

// InfoString returns the info string node for a [FencedCodeBlockKind] block
// or nil otherwise.
func (b *Block) InfoString() *Inline {
	if b.Kind() != FencedCodeBlockKind {
		return nil
	}
	c := b.firstChild().Inline()
	if c.Kind() != InfoStringKind {
		return nil
	}
	return c
}

func (b *Block) firstChild() Node {
	if b.ChildCount() == 0 {
		return Node{}
	}
	return b.Child(0)
}

func (b *Block) lastChild() Node {
	n := b.ChildCount()
	if n == 0 {
		return Node{}
	}
	return b.Child(n - 1)
}

func (b *Block) isOpen() bool {
	return b != nil && b.span.End < 0
}

// close closes b and any open descendents.
// It assumes that only the last child can be open.
// Calling close on a nil block no-ops.
func (b *Block) close(source []byte, parent *Block, end int) {
	if parent != nil && b != parent.lastChild().Block() {
		panic("block to close must be the last child of the parent")
	}
	for ; b.isOpen(); parent, b = b, b.lastChild().Block() {
		b.span.End = end
		if f := blockRules[b.kind].onClose; f != nil {
			replacement := f(source, b)
			parent.blockChildren = append(parent.blockChildren[:len(parent.blockChildren)-1], replacement...)
		}
	}
}

// BlockKind is an enumeration of values returned by [*Block.Kind].
type BlockKind uint16

const (
	// ParagraphKind is used for a block of text.
	ParagraphKind BlockKind = 1 + iota
	// ThematicBreakKind is used for a thematic break, also known as a horizontal rule.
	// It will not contain children.
	ThematicBreakKind
	// ATXHeadingKind is used for headings that start with hash marks.
	ATXHeadingKind
	// SetextHeadingKind is used for headings that end with a divider.
	SetextHeadingKind
	// IndentedCodeBlockKind is used for code blocks started by indentation.
	IndentedCodeBlockKind
	// FencedCodeBlockKind is used for code blocks started by backticks or tildes.
	FencedCodeBlockKind
	// HTMLBlockKind is used for blocks of raw HTML.
	// It should not be wrapped by any tags in rendered HTML output.
	HTMLBlockKind
	// LinkReferenceDefinitionKind is used for a [link reference definition].
	// The first child is always a [LinkLabelKind],
	// the second child is always a [LinkDestinationKind],
	// and it may end with an optional [LinkTitleKind].
	//
	// [link reference definition]: https://spec.commonmark.org/0.30/#link-reference-definition
	LinkReferenceDefinitionKind
	// BlockQuoteKind is used for block quotes.
	BlockQuoteKind
	// ListItemKind is used for items in an ordered or unordered list.
	// The first child will always be of [ListMarkerKind].
	// If the item contains a paragraph and the item is "tight",
	// then the paragraph tag should be stripped.
	ListItemKind
	// ListKind is used for ordered or unordered lists.
	ListKind
	// ListMarkerKind is used to contain the marker in a [ListItemKind] node.
	// It is typically not rendered directly.
	ListMarkerKind

	documentKind
)

// IsCode reports whether the kind is [IndentedCodeBlockKind] or [FencedCodeBlockKind].
func (k BlockKind) IsCode() bool {
	return k == IndentedCodeBlockKind || k == FencedCodeBlockKind
}

// IsHeading reports whether the kind is [ATXHeadingKind] or [SetextHeadingKind].
func (k BlockKind) IsHeading() bool {
	return k == ATXHeadingKind || k == SetextHeadingKind
}

// lineParser is a cursor on a line of text,
// used while splitting a document into blocks.
//
// Exported methods on lineParser
// represent the contract between BlockParser and the rules.
// In the future, lineParser could be exported to permit custom block rules,
// but it's unclear how often this is needed.
type lineParser struct {
	source    []byte
	root      Block
	container *Block

	lineStart    int // number of bytes from beginning of root block to start of line
	line         []byte
	i            int  // byte position within line
	col          int  // 0-based column position within line
	tabRemaining int8 // number of columns left within current tab character

	state int8
}

// Line parser states.
const (
	// stateOpening is the initial state used in [openNewBlocks].
	stateOpening = iota
	// stateOpenMatched is a state used in [openNewBlocks].
	// It is entered from [stateOpening] after the parser has moved its position
	// or the AST has been modified.
	stateOpenMatched
	// stateLineConsumed is a terminal state used in [openNewBlocks].
	// It is entered from [stateOpening]
	// after [*lineParser.ConsumeLine] has been called.
	stateLineConsumed
	// stateDescending is the initial state used in [descendOpenBlocks].
	// No modification of the AST is permitted in this state.
	stateDescending
	// stateDescendTerminated is a terminal state used in [descendOpenBlocks].
	// It is entered from [stateDescending]
	// after [*lineParser.ConsumeLine] has been called.
	// No modification of the AST is permitted in this state.
	stateDescendTerminated
)

func newLineParser(children []*Block, lineStart int, source []byte) *lineParser {
	p := &lineParser{
		root: Block{
			kind:          documentKind,
			span:          Span{Start: 0, End: -1},
			blockChildren: children,
		},
	}
	p.reset(lineStart, source)
	return p
}

func (p *lineParser) reset(lineStart int, newSource []byte) {
	p.lineStart = lineStart
	p.source = newSource
	p.line = newSource[lineStart:]
	p.i = 0
	p.col = 0
	p.container = &p.root
	p.updateTabRemaining()
}

// BytesAfterIndent returns the bytes
// after any indentation immediately following the cursor.
func (p *lineParser) BytesAfterIndent() []byte {
	return bytes.TrimLeft(p.line[p.i:], " \t")
}

// IsRestBlank reports whether the rest of the line is blank.
func (p *lineParser) IsRestBlank() bool {
	return isBlankLine(p.line[p.i:])
}

// Advance advances the parser by n bytes.
// It panics if n is greater than the number of bytes remaining in the line.
func (p *lineParser) Advance(n int) {
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

func (p *lineParser) updateTabRemaining() {
	if p.i < len(p.line) && p.line[p.i] == '\t' {
		p.tabRemaining = int8(columnWidth(p.col, p.line[p.i:p.i+1]))
	} else {
		p.tabRemaining = 0
	}
}

// ConsumeLine advances the cursor past the end of the line.
// This will skip processing line text,
// and additionally close the block when called during block matching.
func (p *lineParser) ConsumeLine() {
	p.Advance(len(p.line) - p.i)
	switch p.state {
	case stateOpening, stateOpenMatched:
		p.state = stateLineConsumed
	case stateDescending:
		p.state = stateDescendTerminated
	}
}

// Indent returns the number of columns of whitespace
// present after the cursor's position.
func (p *lineParser) Indent() int {
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
func (p *lineParser) ConsumeIndent(n int) {
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

// ContainerKind returns the kind of the container block.
func (p *lineParser) ContainerKind() BlockKind {
	return p.container.kind
}

// MorphSetext changes the kind of the container block to [SetextHeadingKind].
func (p *lineParser) MorphSetext(level int) {
	p.container.kind = SetextHeadingKind
	p.container.n = level
}

// TipKind returns the kind of the deepest open block.
func (p *lineParser) TipKind() BlockKind {
	return findTip(&p.root).kind
}

func (p *lineParser) ContainerListDelim() byte {
	if k := p.ContainerKind(); k != ListKind && k != ListItemKind {
		return 0
	}
	return p.container.char
}

// ContainerIndent returns the indent value assigned to the current block.
// Only valid while matching continuation lines.
func (p *lineParser) ContainerIndent() int {
	if p.state != stateDescending && p.state != stateDescendTerminated {
		return math.MaxInt
	}
	return p.container.indent
}

func (p *lineParser) ListItemContainerHasChildren() bool {
	return p.ContainerKind() == ListItemKind && p.container.ChildCount() > 1
}

// ContainerCodeFence returns the character and number of characters
// used to start the code fence being currently matched.
func (p *lineParser) ContainerCodeFence() (c byte, n int) {
	if p.ContainerKind() != FencedCodeBlockKind {
		return 0, 0
	}
	return p.container.char, p.container.n
}

func (p *lineParser) ContainerHTMLCondition() int {
	if p.ContainerKind() != HTMLBlockKind {
		return -1
	}
	return p.container.n
}

// OpenBlock starts a new block at the current position.
func (p *lineParser) OpenBlock(kind BlockKind) {
	if kind == ListKind || kind == ListItemKind || kind == FencedCodeBlockKind || kind == HTMLBlockKind || kind.IsHeading() {
		panic("OpenBlock cannot be called with this kind")
	}
	p.openBlock(kind)
}

func (p *lineParser) OpenListBlock(kind BlockKind, delim byte) {
	if kind != ListKind && kind != ListItemKind {
		panic("OpenListBlock must be called with ListKind or ListItemKind")
	}
	p.openBlock(kind)
	p.container.char = delim
}

func (p *lineParser) OpenFencedCodeBlock(fenceChar byte, numChars int) {
	p.openBlock(FencedCodeBlockKind)
	p.container.char = fenceChar
	p.container.n = numChars
}

func (p *lineParser) OpenHeadingBlock(kind BlockKind, level int) {
	if !kind.IsHeading() {
		panic("OpenHeadingBlock must be called with ATXHeadingKind or SetextHeadingKind")
	}
	p.openBlock(kind)
	p.container.n = level
}

func (p *lineParser) OpenHTMLBlock(conditionIndex int) {
	p.openBlock(HTMLBlockKind)
	p.container.n = conditionIndex
}

func (p *lineParser) openBlock(kind BlockKind) {
	switch p.state {
	case stateDescending, stateDescendTerminated:
		panic("OpenBlock cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}

	// Move up the tree until we find a block that can handle the new child.
	for {
		if rule := blockRules[p.ContainerKind()]; rule.canContain != nil && rule.canContain(kind) {
			break
		}
		parent := findParent(&p.root, p.container)
		p.container.close(p.source, parent, p.lineStart)
		p.container = parent
	}

	// Append to the parent's children list.
	p.container.lastChild().Block().close(p.source, p.container, p.lineStart)
	newChild := &Block{
		kind: kind,
		span: Span{
			Start: p.lineStart + p.i,
			End:   -1,
		},
	}
	p.container.blockChildren = append(p.container.blockChildren, newChild)
	p.container = newChild
}

// SetContainerIndent sets the container's indentation.
func (p *lineParser) SetContainerIndent(indent int) {
	switch p.state {
	case stateOpening:
		panic("SetListItemIndent cannot be called before a match")
	case stateDescending, stateDescendTerminated:
		panic("SetListItemIndent cannot be called in this context")
	}
	if k := p.ContainerKind(); k != ListItemKind && k != FencedCodeBlockKind {
		panic("can't set indent for this block type")
	}
	p.container.indent = indent
}

// CollectInline adds a new [UnparsedKind] inline to the container,
// starting at the current position and ending after n bytes.
// If the current position is at the indent,
// the indent is included -- the n bytes do not count the indent.
func (p *lineParser) CollectInline(kind InlineKind, n int) {
	switch p.state {
	case stateDescendTerminated:
		panic("CollectInline cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}

	if indent := p.Indent(); indent > 0 {
		indentStart := p.lineStart + p.i
		p.Advance(indentLength(p.line[p.i:]))
		p.container.inlineChildren = append(p.container.inlineChildren, &Inline{
			kind: IndentKind,
			span: Span{
				Start: indentStart,
				End:   p.lineStart + p.i,
			},
			indent: indent,
		})
	}

	start := p.lineStart + p.i
	p.Advance(n)
	if kind == InfoStringKind {
		node := parseInfoString(p.source, Span{
			Start: start,
			End:   p.lineStart + p.i,
		})
		p.container.inlineChildren = append(p.container.inlineChildren, node)
	} else {
		p.container.inlineChildren = append(p.container.inlineChildren, &Inline{
			kind: kind,
			span: Span{
				Start: start,
				End:   p.lineStart + p.i,
			},
		})
	}
}

// EndBlock ends a block at the current position.
func (p *lineParser) EndBlock() {
	switch p.state {
	case stateDescending, stateDescendTerminated:
		panic("EndBlock cannot be called in this context")
	case stateOpening:
		p.state = stateOpenMatched
	}
	parent := findParent(&p.root, p.container)
	p.container.close(p.source, parent, p.lineStart+p.i)
	p.container = parent
}

// codeBlockIndentLimit is the column width of an indent
// required to start an indented code block.
const codeBlockIndentLimit = 4

const blockQuotePrefix = ">"

var blockStarts = []func(*lineParser){
	// Block quote.
	func(p *lineParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		if !hasBytePrefix(p.BytesAfterIndent(), blockQuotePrefix) {
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
	func(p *lineParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		h := parseATXHeading(p.BytesAfterIndent())
		if h.level < 1 {
			return
		}

		p.ConsumeIndent(indent)
		p.OpenHeadingBlock(ATXHeadingKind, h.level)
		p.Advance(h.content.Start)
		p.CollectInline(UnparsedKind, h.content.Len())
		p.ConsumeLine()
		p.EndBlock()
	},

	// Fenced code block.
	func(p *lineParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		f := parseCodeFence(p.BytesAfterIndent())
		if f.n == 0 {
			return
		}

		p.ConsumeIndent(indent)
		p.OpenFencedCodeBlock(f.char, f.n)
		p.SetContainerIndent(indent)
		if f.info.IsValid() {
			p.Advance(f.info.Start)
			p.CollectInline(InfoStringKind, f.info.Len())
		}
		p.ConsumeLine()
	},

	// HTML block.
	func(p *lineParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		line := p.BytesAfterIndent()
		if len(line) == 0 || line[0] != '<' {
			return
		}
		for i, conds := range htmlBlockConditions {
			if conds.startCondition(line) {
				if !conds.canInterruptParagraph && p.ContainerKind() == ParagraphKind {
					return
				}
				p.OpenHTMLBlock(i)
				if conds.endCondition(line) {
					p.CollectInline(RawHTMLKind, len(p.BytesAfterIndent()))
					p.ConsumeLine()
					p.EndBlock()
				}
				return
			}
		}
	},

	// Setext heading.
	func(p *lineParser) {
		if p.ContainerKind() != ParagraphKind {
			return
		}
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		level := parseSetextHeadingUnderline(p.BytesAfterIndent())
		if level == 0 {
			return
		}
		p.MorphSetext(level)
		p.ConsumeLine()
		p.EndBlock()
	},

	// Thematic break.
	func(p *lineParser) {
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
		p.ConsumeLine()
		p.EndBlock()
	},

	// List item.
	func(p *lineParser) {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return
		}
		m := parseListMarker(p.BytesAfterIndent())
		if m.end < 0 || (p.ContainerKind() == ParagraphKind && m.isOrdered() && m.n != 1) {
			return
		}
		// If this interrupts a paragraph, ensure first line isn't blank.
		if p.ContainerKind() == ParagraphKind && isBlankLine(p.BytesAfterIndent()[m.end:]) {
			return
		}

		p.ConsumeIndent(indent)
		if p.ContainerKind() != ListKind || p.ContainerListDelim() != m.delim {
			p.OpenListBlock(ListKind, m.delim)
		}
		p.OpenListBlock(ListItemKind, m.delim)
		p.OpenBlock(ListMarkerKind)
		p.Advance(m.end)
		p.EndBlock()
		if p.IsRestBlank() {
			p.SetContainerIndent(indent + m.end + 1)
			p.ConsumeLine()
			return
		}
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
		p.SetContainerIndent(indent + m.end + padding)
	},

	// Indented code block.
	func(p *lineParser) {
		if p.Indent() < codeBlockIndentLimit || p.IsRestBlank() || p.TipKind() == ParagraphKind {
			return
		}
		p.ConsumeIndent(codeBlockIndentLimit)
		p.OpenBlock(IndentedCodeBlockKind)
	},
}

type blockRule struct {
	match        func(*lineParser) bool
	onClose      func(source []byte, block *Block) []*Block
	canContain   func(childKind BlockKind) bool
	acceptsLines bool
}

var blockRules = map[BlockKind]blockRule{
	documentKind: {
		match:      func(*lineParser) bool { return true },
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	ListKind: {
		match:      func(*lineParser) bool { return true },
		canContain: func(childKind BlockKind) bool { return childKind == ListItemKind },
		onClose: func(source []byte, block *Block) []*Block {
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
			items := block.blockChildren
		determineLoose:
			for i, item := range items {
				if i < len(items)-1 && endsWithBlankLine(item) {
					block.listLoose = true
					break determineLoose
				}
				subitems := item.blockChildren
				for j, subitem := range subitems {
					if (i < len(items)-1 || j < len(subitems)-1) &&
						endsWithBlankLine(subitem) {
						block.listLoose = true
						break determineLoose
					}
				}
			}
			if block.listLoose {
				for _, item := range items {
					item.listLoose = true
				}
			}
			return []*Block{block}
		},
	},
	ListItemKind: {
		match: func(p *lineParser) bool {
			switch {
			case p.IsRestBlank():
				if !p.ListItemContainerHasChildren() {
					// A list item can begin with at most one blank line.
					return false
				}
				p.ConsumeIndent(p.Indent())
				return true
			case p.Indent() >= p.ContainerIndent():
				p.ConsumeIndent(p.ContainerIndent())
				return true
			default:
				return false
			}
		},
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	BlockQuoteKind: {
		match: func(p *lineParser) bool {
			indent := p.Indent()
			if indent >= codeBlockIndentLimit {
				return false
			}
			if !hasBytePrefix(p.BytesAfterIndent(), blockQuotePrefix) {
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
	FencedCodeBlockKind: {
		match: func(p *lineParser) bool {
			lineIndent := p.Indent()
			if lineIndent < codeBlockIndentLimit {
				startChar, startCharCount := p.ContainerCodeFence()
				f := parseCodeFence(p.BytesAfterIndent())
				if f.n > 0 && !f.info.IsValid() && f.char == startChar && f.n >= startCharCount {
					// Closing fence.
					p.ConsumeLine()
					return false
				}
			}
			if blockIndent := p.ContainerIndent(); lineIndent < blockIndent {
				p.ConsumeIndent(lineIndent)
			} else {
				p.ConsumeIndent(blockIndent)
			}
			return true
		},
		acceptsLines: true,
	},
	IndentedCodeBlockKind: {
		match: func(p *lineParser) bool {
			indent := p.Indent()
			if indent < codeBlockIndentLimit {
				if !p.IsRestBlank() {
					return false
				}
				p.ConsumeIndent(indent)
			} else {
				p.ConsumeIndent(codeBlockIndentLimit)
			}
			return true
		},
		onClose: func(source []byte, block *Block) []*Block {
			// "Blank lines preceding or following an indented code block are not included in it."
			for i := block.ChildCount() - 1; i >= 0; i-- {
				child := block.inlineChildren[i]
				if child.Kind() != TextKind || !isBlankLine(spanSlice(source, child.Span())) {
					break
				}
				block.inlineChildren[i] = nil // free for GC
				block.inlineChildren = block.inlineChildren[:i:i]
			}
			return []*Block{block}
		},
		acceptsLines: true,
	},
	ATXHeadingKind: {
		acceptsLines: true,
	},
	HTMLBlockKind: {
		match: func(p *lineParser) bool {
			if htmlBlockConditions[p.ContainerHTMLCondition()].endCondition(p.BytesAfterIndent()) {
				if !p.IsRestBlank() {
					p.CollectInline(RawHTMLKind, len(p.BytesAfterIndent()))
				}
				p.ConsumeLine()
				return false
			}
			return true
		},
		acceptsLines: true,
	},
	ParagraphKind: {
		match: func(p *lineParser) bool {
			return !p.IsRestBlank()
		},
		acceptsLines: true,
		onClose:      onCloseParagraph,
	},
	SetextHeadingKind: {
		onClose: onCloseParagraph,
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
	level   int // 1-6
	content Span
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
		h.content = Span{Start: i, End: i}
		return h
	}
	if !(line[i] == ' ' || line[i] == '\t') {
		return atxHeading{}
	}
	i++

	// Advance past leading whitespace.
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	h.content.Start = i

	// Find end of heading line. Skip past trailing spaces.
	h.content.End = len(line)
	hitHash := false
scanBack:
	for ; h.content.End > h.content.Start; h.content.End-- {
		switch line[h.content.End-1] {
		case '\r', '\n':
			// Skip past EOL.
		case ' ', '\t':
			if isEndEscaped(line[:h.content.End-1]) {
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
	for i := h.content.End - 1; ; i-- {
		if i <= h.content.Start {
			h.content.End = h.content.Start
			break
		}
		switch line[i] {
		case '#':
			// Keep going.
		case ' ', '\t':
			h.content.End = i + 1
			break scanTrailingHashes
		default:
			return h
		}
	}
	// We've hit the end of hashmarks. Trim trailing whitespace.
	for ; h.content.End > h.content.Start; h.content.End-- {
		if b := line[h.content.End-1]; !(b == ' ' || b == '\t') || isEndEscaped(line[:h.content.End-1]) {
			break
		}
	}
	return h
}

// parseSetextHeadingUnderline returns the line's heading level
// if it is a [setext heading underline],
// or zero otherwise.
// parseSetextHeadingUnderline assumes that the caller has stripped any leading indentation.
//
// [setext heading underline]: https://spec.commonmark.org/0.30/#setext-heading-underline
func parseSetextHeadingUnderline(line []byte) (level int) {
	if len(line) == 0 {
		return 0
	}
	switch line[0] {
	case '=':
		level = 1
	case '-':
		level = 2
	default:
		return 0
	}
	for i := 1; i < len(line); i++ {
		if line[i] != line[0] {
			if !isBlankLine(line[i:]) {
				return 0
			}
			return level
		}
	}
	return level
}

type codeFence struct {
	char byte // either '`' or '~'
	n    int
	info Span
}

// parseCodeFence attempts to parse a [code fence] at the beginning of the line.
// [codeFence.n] is 0 if the line does not begin with a marker.
// parseCodeFence assumes that the caller has stripped any leading indentation.
//
// [code fence]: https://spec.commonmark.org/0.30/#code-fence
func parseCodeFence(line []byte) codeFence {
	const minConsecutive = 3
	if len(line) < minConsecutive || (line[0] != '`' && line[0] != '~') {
		return codeFence{info: NullSpan()}
	}
	f := codeFence{
		char: line[0],
		n:    1,
		info: NullSpan(),
	}
	for f.n < len(line) && line[f.n] == f.char {
		f.n++
	}
	if f.n < minConsecutive {
		return codeFence{info: NullSpan()}
	}
	for i := f.n; i < len(line) && f.info.Start < 0; i++ {
		if c := line[i]; !isSpaceTabOrLineEnding(c) {
			f.info.Start = i
		}
	}
	if f.info.Start >= 0 {
		// Trim trailing whitespace.
		for f.info.End = len(line); f.info.End > f.info.Start; f.info.End-- {
			if c := line[f.info.End-1]; !isSpaceTabOrLineEnding(c) {
				break
			}
		}

		// "If the info string comes after a backtick fence,
		// it may not contain any backtick characters."
		if f.char == '`' {
			for i := f.info.Start; i < f.info.End; i++ {
				if line[i] == '`' {
					return codeFence{info: NullSpan()}
				}
			}
		}
	}
	return f
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

// onCloseParagraph handles the closing of a paragraph block or a [SetextHeadingBlock]
// by searching its beginning for link reference definitions.
func onCloseParagraph(source []byte, originalBlock *Block) []*Block {
	if len(originalBlock.inlineChildren) == 0 {
		return []*Block{originalBlock}
	}

	contentStart := originalBlock.inlineChildren[0].Span().Start
	var setextOrphanParagraph *Block
	if originalBlock.Kind() == SetextHeadingKind {
		blockStart := originalBlock.inlineChildren[len(originalBlock.inlineChildren)-1].Span().End
		lineStart := blockStart
		for source[lineStart] == ' ' || source[lineStart] == '\t' {
			lineStart++
		}
		setextOrphanParagraph = &Block{
			kind: ParagraphKind,
			span: Span{
				Start: blockStart,
				End:   -1,
			},
			inlineChildren: []*Inline{{
				kind: UnparsedKind,
				span: Span{
					Start: lineStart,
					End:   originalBlock.Span().End,
				},
			}},
		}
	}
	r := newInlineByteReader(source, originalBlock.inlineChildren, contentStart)
	var result []*Block
	for {
		// At a minimum, a link reference definition must have a label and a destination.
		label := parseLinkLabel(r)
		if !label.span.IsValid() {
			return append(result, originalBlock)
		}
		if r.current() != ':' {
			return append(result, originalBlock)
		}
		r.next()
		if !skipLinkSpace(r) {
			return append(result, originalBlock)
		}
		destination := parseLinkDestination(r)
		if !destination.span.IsValid() {
			return append(result, originalBlock)
		}

		// Check if we're at the end of the line.
		// The title may be on the next line,
		// but if it isn't, then we may need to back up.
		sepPoint := r.pos
		destinationEOL := readEOL(r)
		cloned := *r
		if destinationEOL < 0 && r.pos == sepPoint && r.current() != 0 {
			// Title must be separated by at least one space.
			// Abandon this definition.
			return append(result, originalBlock)
		}

		// We likely have a new link reference definition, so prep it.
		newBlock := &Block{
			kind: LinkReferenceDefinitionKind,
			span: Span{Start: label.span.Start, End: destination.span.End},
		}

		labelInline := &Inline{
			kind: LinkLabelKind,
			span: label.inner,
			ref:  transformLinkReferenceSpan(source, originalBlock.inlineChildren, label.inner),
		}
		collectLinkLabelText(
			labelInline,
			newInlineByteReader(source, originalBlock.inlineChildren, label.inner.Start),
			label.inner.End,
		)
		newBlock.inlineChildren = append(newBlock.inlineChildren, labelInline)

		destinationInline := &Inline{
			kind: LinkDestinationKind,
			span: destination.span,
		}
		collectLinkAttributeText(
			destinationInline,
			newInlineByteReader(source, originalBlock.inlineChildren, destination.text.Start),
			destination.text.End,
		)
		newBlock.inlineChildren = append(newBlock.inlineChildren, destinationInline)

		// Consume whitespace before the title (only if we moved to a subsequent line).
		if !skipLinkSpace(r) {
			// We hit EOF before encountering anything else.
			newBlock.span.End = destinationEOL
			result = append(result, newBlock)
			if setextOrphanParagraph != nil {
				result = append(result, setextOrphanParagraph)
			}
			return result
		}

		// Parse title, if present.
		title := parseLinkTitle(r)
		if !title.span.IsValid() {
			if destinationEOL < 0 {
				// There were non-space characters after the destination and it wasn't a title:
				// abandon this definition.
				return append(result, originalBlock)
			}

			// We have a complete link reference definition,
			// we just need to rewind to the beginning of the line.
			newBlock.span.End = destinationEOL
			result = append(result, newBlock)

			originalBlock.span.Start = cloned.pos
			firstChild := nodeIndexForPosition(originalBlock.inlineChildren, cloned.pos)
			if firstChild < 0 {
				if setextOrphanParagraph != nil {
					result = append(result, setextOrphanParagraph)
				}
				return result
			}
			originalBlock.inlineChildren = originalBlock.inlineChildren[firstChild:]
			*r = cloned
			continue
		}

		// We have a title. Now ensure there are no more characters on the line.
		titleEOL := readEOL(r)
		if titleEOL < 0 {
			// No dice. If the title wasn't cleanly on another line,
			// then we don't have a definition.
			if destinationEOL < 0 {
				return append(result, originalBlock)
			}

			// We have a valid link reference definition without a title.
			// Any line that starts with a valid title
			// can't possibly be another link reference definition
			// (titles can't start with a '['),
			// so we know we're done.
			newBlock.span.End = destinationEOL
			result = append(result, newBlock)

			originalBlock.span.Start = cloned.pos
			firstChild := nodeIndexForPosition(originalBlock.inlineChildren, cloned.pos)
			if firstChild < 0 {
				if setextOrphanParagraph != nil {
					result = append(result, setextOrphanParagraph)
				}
				return result
			}
			originalBlock.inlineChildren = originalBlock.inlineChildren[firstChild:]
			return append(result, originalBlock)
		}

		// We now have a link reference definition with all three parts:
		// label, destination, and title.
		// Collect it up, shorten the block, and loop through again.
		titleInline := &Inline{
			kind: LinkTitleKind,
			span: title.span,
		}
		collectLinkAttributeText(
			titleInline,
			newInlineByteReader(source, originalBlock.inlineChildren, title.text.Start),
			title.text.End,
		)
		newBlock.inlineChildren = append(newBlock.inlineChildren, titleInline)
		newBlock.span.End = titleEOL
		result = append(result, newBlock)

		originalBlock.span.Start = r.pos
		firstChild := nodeIndexForPosition(originalBlock.inlineChildren, r.pos)
		if firstChild < 0 {
			if setextOrphanParagraph != nil {
				result = append(result, setextOrphanParagraph)
			}
			return result
		}
		originalBlock.inlineChildren = originalBlock.inlineChildren[firstChild:]
	}
}

// skipLinkSpace skips over spaces and tabs
// and returns whether it stopped before EOF.
func skipSpacesAndTabs(r *inlineByteReader) bool {
	for r.current() == ' ' || r.current() == '\t' {
		if !r.next() {
			return false
		}
	}
	return r.current() != 0
}

// readEOL skips over any trailing spaces
// and then attempts to consume a single line ending or EOF,
// returning the position of the line end (exclusive).
// If unsuccessful, readEOL will have consumed the whitespace
// and returns -1.
func readEOL(r *inlineByteReader) (end int) {
	if !skipSpacesAndTabs(r) {
		return r.pos
	}
	switch r.current() {
	case '\r':
		if !r.next() {
			return r.prevPos + 1
		}
		if r.current() == '\n' {
			r.next()
		}
		return r.prevPos + 1
	case '\n':
		r.next()
		return r.prevPos + 1
	default:
		return -1
	}
}
