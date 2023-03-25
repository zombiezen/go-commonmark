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

import "unsafe"

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

func (b *Block) AsNode() Node {
	if b == nil {
		return Node{}
	}
	return Node{
		typ: nodeTypeBlock,
		ptr: unsafe.Pointer(b),
	}
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
func (b *Block) close(end int) {
	for ; b.isOpen(); b = b.lastChild().Block() {
		b.end = end
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

	lineStart int // number of bytes from beginning of root block to start of line
	line      []byte
	i         int  // byte position within line
	tabpos    int8 // column position within current tab character
	opening   bool
}

// Bytes returns the bytes remaining in the line.
func (bp *blockParser) Bytes() []byte {
	return bp.line[bp.i:]
}

// Advance advances the parser by n bytes.
// It panics if n is greater than the number of bytes remaining in the line.
func (bp *blockParser) Advance(n int) {
	newIndex := bp.i + n
	if newIndex > len(bp.line) {
		panic("index out of bounds")
	}
	bp.i = newIndex
	bp.tabpos = 0
}

// Indent returns the number of columns of whitespace
// present after the cursor's position.
func (bp *blockParser) Indent() int {
	if bp.i >= len(bp.line) {
		return 0
	}
	var indent int
	switch bp.line[bp.i] {
	case ' ':
		indent = 1
	case '\t':
		indent = tabStopSize - int(bp.tabpos)
	default:
		return 0
	}
	for _, c := range bp.line[bp.i+1:] {
		switch c {
		case ' ':
			indent++
		case '\t':
			indent += tabStopSize
		default:
			return indent
		}
	}
	return indent
}

// ConsumeIndent advances the parser by n columns of whitespace.
// It panics if n is greater than bp.Indent().
func (bp *blockParser) ConsumeIndent(n int) {
	for n > 0 {
		switch {
		case bp.i < len(bp.line) && bp.line[bp.i] == ' ':
			n--
			bp.i++
		case bp.i < len(bp.line) && bp.line[bp.i] == '\t':
			if n < tabStopSize-int(bp.tabpos) {
				bp.tabpos += int8(n)
				return
			}
			n -= tabStopSize - int(bp.tabpos)
			bp.i++
			bp.tabpos = 0
		default:
			panic("consumed past end of indent")
		}
	}
}

func (bp *blockParser) ContainerKind() BlockKind {
	if bp.container == nil {
		return documentKind
	}
	return bp.container.kind
}

// OpenBlock starts a new block at the current position.
func (bp *blockParser) OpenBlock(kind BlockKind) {
	if !bp.opening {
		panic("OpenBlock cannot be called in this context")
	}
	if bp.container == nil && bp.root.kind != 0 {
		// Close the root block.
		bp.root.end = bp.lineStart
		return
	}
	_, bp.container = appendNewBlock(bp.root, bp.container, kind, bp.lineStart, bp.lineStart+bp.i)
}

// CollectInline adds a new [UnparsedKind] inline to the current block,
// starting at the current position and ending after n bytes.
func (bp *blockParser) CollectInline(n int) {
	if !bp.opening {
		panic("CollectInline cannot be called in this context")
	}
	start := bp.lineStart + bp.i
	bp.Advance(n)
	if bp.container == nil {
		return
	}
	bp.container.children = append(bp.container.children, (&Inline{
		kind:  UnparsedKind,
		start: start,
		end:   bp.lineStart + bp.i,
	}).AsNode())
}

// EndBlock ends a block at the current position.
func (bp *blockParser) EndBlock() {
	if !bp.opening {
		panic("EndBlock cannot be called in this context")
	}
	if bp.container == nil {
		if bp.root.end < 0 {
			bp.root.close(bp.lineStart + bp.i)
		}
		return
	}
	bp.container.close(bp.lineStart + bp.i)
	bp.container = findParent(bp.root, bp.container)
}

type parseResult int8

const (
	noMatch parseResult = iota
	matched
	matchedEntireLine
)

// codeBlockIndentLimit is the column width of an indent
// required to start an indented code block.
const codeBlockIndentLimit = 4

var blockStarts = []func(*blockParser) parseResult{
	// Block quote.
	func(p *blockParser) parseResult {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return noMatch
		}
		end := parseBlockQuote(trimIndent(p.Bytes()))
		if end < 0 {
			return noMatch
		}
		p.ConsumeIndent(indent)
		p.OpenBlock(BlockQuoteKind)
		p.Advance(end)
		p.EndBlock()
		return matched
	},

	// ATX heading.
	func(p *blockParser) parseResult {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return noMatch
		}
		h := parseATXHeading(trimIndent(p.Bytes()))
		if h.level < 1 {
			return noMatch
		}
		p.ConsumeIndent(indent)
		p.OpenBlock(ATXHeadingKind)
		p.Advance(h.contentStart)
		p.CollectInline(h.contentEnd - h.contentStart)
		p.Advance(len(p.Bytes()))
		p.EndBlock()
		return matchedEntireLine
	},

	// Thematic break.
	func(p *blockParser) parseResult {
		indent := p.Indent()
		if indent >= codeBlockIndentLimit {
			return noMatch
		}
		end := parseThematicBreak(trimIndent(p.Bytes()))
		if end < 0 {
			return noMatch
		}
		p.ConsumeIndent(indent)
		p.OpenBlock(ThematicBreakKind)
		p.Advance(end)
		p.EndBlock()
		return matchedEntireLine
	},
	// Indented code block.
	func(p *blockParser) parseResult {
		if p.ContainerKind() == ParagraphKind {
			return noMatch
		}
		indent := p.Indent()
		if indent < codeBlockIndentLimit || isBlankLine(p.Bytes()) {
			return noMatch
		}
		p.ConsumeIndent(codeBlockIndentLimit)
		p.OpenBlock(IndentedCodeBlockKind)
		return matched
	},
}

type blockRule struct {
	match        func(*blockParser) parseResult
	canContain   func(childKind BlockKind) bool
	acceptsLines bool
}

var blocks = map[BlockKind]blockRule{
	documentKind: {
		match:      func(*blockParser) parseResult { return matched },
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	BlockQuoteKind: {
		match: func(p *blockParser) parseResult {
			indent := p.Indent()
			if indent >= codeBlockIndentLimit {
				return noMatch
			}
			end := parseBlockQuote(trimIndent(p.Bytes()))
			if end < 0 {
				return noMatch
			}
			p.ConsumeIndent(indent)
			p.Advance(end)
			return matched
		},
		canContain: func(childKind BlockKind) bool { return childKind != ListItemKind },
	},
	IndentedCodeBlockKind: {
		match: func(p *blockParser) parseResult {
			if b := p.Bytes(); isBlankLine(b) {
				p.Advance(len(b))
				return matchedEntireLine
			}
			indent := p.Indent()
			if indent < codeBlockIndentLimit {
				return noMatch
			}
			p.ConsumeIndent(codeBlockIndentLimit)
			return matched
		},
		acceptsLines: true,
	},
	ParagraphKind: {
		match: func(p *blockParser) parseResult {
			if isBlankLine(p.Bytes()) {
				return noMatch
			}
			return matched
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

// parseBlockQuote attempts to parse a [block quote marker] from the beginning of the line.
// It returns the end of the block quote marker
// or -1 if the line does not begin with the marker.
// parseBlockQuote assumes that the caller has stripped any leading indentation.
//
// [block quote marker]: https://spec.commonmark.org/0.30/#block-quote-marker
func parseBlockQuote(line []byte) (end int) {
	if len(line) == 0 || line[0] != '>' {
		return -1
	}
	if len(line) > 1 && line[1] == ' ' {
		return 2
	}
	return 1
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
