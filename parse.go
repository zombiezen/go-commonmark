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

// Package markdown provides a [CommonMark] parser.
//
// [CommonMark]: https://commonmark.org/
package markdown

import (
	"bytes"
	"fmt"
	"io"
)

const (
	tabStopSize          = 4
	codeBlockIndentLimit = 4
)

type Parser struct {
	buf      []byte // current block being parsed
	offset   int64  // offset from beginning of stream to beginning of buf
	parsePos int    // parse position within buf
	lineno   int    // line number of parse position

	r   io.Reader
	err error // non-nil indicates there is no more data after end of buf
}

func NewParser(r io.Reader) *Parser {
	return &Parser{
		r: r,
	}
}

func Parse(source []byte) []*RootBlock {
	if bytes.IndexByte(source, 0) >= 0 {
		// Contains one or more NUL bytes.
		// Replace with Unicode replacement character.
		source = bytes.ReplaceAll(source, []byte{0}, []byte("\ufffd"))
	}
	p := &Parser{
		buf: source,
		err: io.EOF,
	}
	var blocks []*RootBlock
	for {
		block, err := p.NextBlock()
		if err == io.EOF {
			return blocks
		}
		if err != nil {
			panic(err)
		}
		blocks = append(blocks, block)
	}
}

func (p *Parser) NextBlock() (*RootBlock, error) {
	// Keep going until we encounter a non-blank line.
	var line []byte
	for {
		line = p.readline()
		if len(line) == 0 {
			return nil, p.err
		}
		if !isBlankLine(line) {
			break
		}

		p.offset += int64(p.parsePos)
		p.buf = p.buf[p.parsePos:]
		p.parsePos = 0
	}

	// Open root block.
	root := &RootBlock{
		StartLine:   p.lineno,
		StartOffset: p.offset,
		Block: Block{
			end: -1,
		},
	}
	lastOpenBlock, rest := openNewBlocks(root, nil, true, line, 0, 0)
	if !root.isOpen() {
		// Single-line block.
		root.Source = p.consume()
		return root, nil
	}
	addLineText(root, lastOpenBlock, line, 0, len(line)-len(rest))

	// Parse subsequent lines.
	for {
		lineStart := p.parsePos
		line = p.readline()
		container, rest, allMatched := descendOpenBlocks(root, line)
		lastOpenBlock, rest := openNewBlocks(root, container, allMatched, rest, lineStart, lineStart+len(line)-len(rest))
		if lastOpenBlock == nil {
			if !isBlankLine(rest) {
				// If there's remaining text on the line,
				// then rewind to the beginning of the line.
				p.parsePos = lineStart
			}
			root.Source = p.consume()
			return root, nil
		}
		addLineText(root, lastOpenBlock, rest, lineStart, p.parsePos-len(rest))
	}
}

// descendOpenBlocks iterates through the open blocks,
// starting at the top-level block,
// and descending through last children down to the last open block.
// It returns the last matched block
// or nil if not even the top-level block could be matched.
//
// This corresponds to the first step of [Phase 1]
// in the CommonMark recommended parsing strategy.
//
// [Phase 1]: https://spec.commonmark.org/0.30/#phase-1-block-structure
func descendOpenBlocks(root *RootBlock, line []byte) (container *Block, rest []byte, allMatched bool) {
	var parent *Block
	container = &root.Block

	for {
		indent, pos := consumeIndent(line)

		switch parserBlockKind(container) {
		case BlockQuoteKind:
			if indent >= codeBlockIndentLimit {
				return parent, line, false
			}
			end := parseBlockQuote(line[pos:])
			if end < 0 {
				return parent, line, false
			}
			line = line[pos+end:]
		case IndentedCodeBlockKind:
			if indent < codeBlockIndentLimit {
				return parent, line, false
			}
			// Include entire indent so that we can interpret spacing later.
			return container, line, false
		case ParagraphKind:
			if isBlankLine(line) {
				return parent, line, false
			}
		default:
			panic("unreachable")
		}

		lastChild := container.lastChild().Block()
		if !lastChild.isOpen() {
			return container, line, true
		}
		parent, container = container, lastChild
	}
}

// openNewBlocks looks for new block starts,
// closing any blocks unmatched in step 1
// before creating new blocks as descendants of the last matched container block.
// A nil container is interpreted as the document being the last matched container block.
// openNewBlocks returns the deepest open block and any unprocessed text from the line.
//
// This corresponds to the second step of [Phase 1]
// in the CommonMark recommended parsing strategy.
//
// [Phase 1]: https://spec.commonmark.org/0.30/#phase-1-block-structure
func openNewBlocks(root *RootBlock, container *Block, allMatched bool, remaining []byte, lineStart, remainingStart int) (lastOpenBlock *Block, newRemaining []byte) {
	if lineStart == remainingStart+len(remaining) {
		// Special case: EOF. Close the root block.
		root.close(lineStart)
		return nil, nil
	}

	containerKind := parserBlockKind(container)

	addBlock := func(kind BlockKind, start int) (parent, newChild *Block) {
		if container == nil && root.kind != 0 {
			// Close the root block.
			root.end = lineStart
			return nil, nil
		}
		parent, newChild = appendNewBlock(root, container, kind, lineStart, remainingStart+start)
		container = newChild
		containerKind = parserBlockKind(container)
		return parent, newChild
	}

	for root.isOpen() &&
		containerKind != FencedCodeBlockKind &&
		containerKind != IndentedCodeBlockKind &&
		containerKind != HTMLBlockKind {
		indent, pos := consumeIndent(remaining)
		if indent >= codeBlockIndentLimit {
			if containerKind == ParagraphKind || isBlankLine(remaining[pos:]) {
				break
			}
			addBlock(IndentedCodeBlockKind, 0)
			continue
		} else if end := parseBlockQuote(remaining[pos:]); end >= 0 {
			addBlock(BlockQuoteKind, pos)
			remaining = remaining[end:]
			remainingStart += end
		} else if h := parseATXHeading(remaining[pos:]); h.level != 0 {
			newBlockParent, newBlock := addBlock(ATXHeadingKind, pos)
			if newBlock == nil {
				return nil, remaining[pos:]
			}
			newBlock.end = remainingStart + len(remaining)
			newBlock.children = append(newBlock.children, (&Inline{
				kind:  UnparsedKind,
				start: remainingStart + pos + h.contentStart,
				end:   remainingStart + pos + h.contentEnd,
			}).AsNode())
			return newBlockParent, nil
		} else if end := parseThematicBreak(remaining[pos:]); end >= 0 && !(containerKind == ParagraphKind && !allMatched) {
			newBlockParent, newBlock := addBlock(ThematicBreakKind, pos)
			if newBlock == nil {
				return nil, remaining[pos:]
			}
			newBlock.end = remainingStart + pos + end
			return newBlockParent, nil
		} else {
			// Hit the text.
			break
		}
	}

	return container, remaining
}

// appendNewBlock creates a new block and appends it to the tree,
// preferring insertion at the given parent block.
// The parent block is assumed to be open; the results are undefined if it is closed.
// If the parent block can't contain a block of the given kind,
// it will be closed and appendNewBlock will look up the tree
// to find a block that supports the new block kind.
// newNode will be nil if and only if appendNewBlock could not find a suitable parent.
func appendNewBlock(root *RootBlock, parent *Block, kind BlockKind, lineStart, start int) (actualParent, newNode *Block) {
	// Move up the tree until we find a block that can handle the new child.
	for {
		parentKind := parserBlockKind(parent)
		if parent == nil {
			parentKind = documentKind
		}
		if parentKind.canContain(kind) {
			break
		}
		parent.close(lineStart)
		if parent == nil {
			return nil, nil
		}
		parent = findParent(root, parent)
	}

	// Special case: parent is the document.
	if parent == nil {
		if root.kind != 0 {
			return nil, nil
		}
		root.kind = kind
		root.start = start
		root.end = -1
		return nil, &root.Block
	}

	// Normal case: append to the parent's children list.
	parent.lastChild().Block().close(lineStart)
	newChild := &Block{
		kind:  kind,
		start: start,
		end:   -1,
	}
	parent.children = append(parent.children, newChild.AsNode())
	return parent, newChild
}

func addLineText(root *RootBlock, container *Block, remaining []byte, lineStart, remainingStart int) {
	containerKind := parserBlockKind(container)
	if containerKind == FencedCodeBlockKind || containerKind == IndentedCodeBlockKind {
		_, indentEnd := consumeIndent(remaining)
		container.children = append(container.children,
			(&Inline{
				kind:  CodeIndentKind,
				start: remainingStart,
				end:   remainingStart + indentEnd,
			}).AsNode(),
			(&Inline{
				kind:  UnparsedKind,
				start: remainingStart + indentEnd,
				end:   remainingStart + len(remaining),
			}).AsNode(),
		)
	} else if containerKind.acceptsLines() {
		container.children = append(container.children, (&Inline{
			kind:  UnparsedKind,
			start: remainingStart,
			end:   remainingStart + len(remaining),
		}).AsNode())
	} else {
		// Create paragraph container for line.
		_, p := appendNewBlock(root, container, ParagraphKind, lineStart, remainingStart)
		p.children = append(p.children, (&Inline{
			kind:  UnparsedKind,
			start: remainingStart,
			end:   remainingStart + len(remaining),
		}).AsNode())
	}
}

// parserBlockKind is similar to [*Block.Kind]
// but returns documentKind for nil
// to accomodate the pattern of using a nil parent to represent the document.
func parserBlockKind(b *Block) BlockKind {
	if b == nil {
		return documentKind
	}
	return b.kind
}

func findParent(root *RootBlock, b *Block) *Block {
	for parent, curr := (*Block)(nil), &root.Block; ; {
		if curr == nil {
			return nil
		}
		if curr == b {
			return parent
		}
		parent = curr
		curr = curr.lastChild().Block()
	}
}

// readline reads the next line of input, growing p.buf as necessary.
// It will return a zero-length slice if and only if it has reached the end of input.
// After calling readline, p.lineno will contain the current line's number.
func (p *Parser) readline() []byte {
	const (
		chunkSize    = 8 * 1024
		maxBlockSize = 1024 * 1024
	)

	eolEnd := -1
	for {
		// Check if we have a line ending available.
		if i := bytes.IndexAny(p.buf[p.parsePos:], "\r\n"); i >= 0 {
			eolStart := p.parsePos + i
			if p.buf[eolStart] == '\n' {
				eolEnd = eolStart + 1
				break
			}
			if eolStart+1 < len(p.buf) {
				// Carriage return with enough buffer for 1 byte lookahead.
				eolEnd = eolStart + 1
				if p.buf[eolEnd] == '\n' {
					eolEnd++
				}
				break
			}
			if p.err != nil {
				// Carriage return right before EOF.
				eolEnd = len(p.buf)
				break
			}
		}

		// If we don't have any more line ending available,
		// but we're at EOF, return everything we have.
		if p.err != nil {
			eolEnd = len(p.buf)
			break
		}

		// If we're already at the maximum block size,
		// then drop the line and pretend it's an EOF.
		if len(p.buf) >= maxBlockSize {
			p.lineno++
			p.buf = p.buf[:p.parsePos]
			p.err = fmt.Errorf("line %d: block too large", p.lineno)
			return nil
		}

		// Grab more data from the reader.
		newSize := len(p.buf) + chunkSize
		if newSize > maxBlockSize {
			newSize = maxBlockSize
		}
		if cap(p.buf) < newSize {
			newbuf := make([]byte, len(p.buf), newSize)
			copy(newbuf, p.buf)
			p.buf = newbuf
		}
		var n int
		n, p.err = p.r.Read(p.buf[len(p.buf):newSize])
		p.buf = p.buf[:len(p.buf)+n]
	}

	line := p.buf[p.parsePos:eolEnd]
	p.parsePos = eolEnd
	p.lineno++
	return line
}

func (p *Parser) consume() []byte {
	out := p.buf[:p.parsePos:p.parsePos]
	p.offset += int64(p.parsePos)
	p.buf = p.buf[p.parsePos:]
	p.parsePos = 0
	return out
}

func consumeIndent(line []byte) (indent, nbytes int) {
	for ; nbytes < len(line); nbytes++ {
		if line[nbytes] == ' ' {
			indent++
		} else if line[nbytes] == '\t' {
			indent += tabStopSize
		} else {
			break
		}
	}
	return
}

func isBlankLine(line []byte) bool {
	for _, b := range line {
		if !(b == '\r' || b == '\n' || b == ' ' || b == '\t') {
			return false
		}
	}
	return true
}

// parseThematicBreak attempts to parse the line as a [thematic break].
// It returns the end of the thematic break characters
// or -1 if the line is not a thematic break.
// parseThematicBreak assumes that the caller has stripped any leading indentation.
//
// [thematic break]: https://spec.commonmark.org/0.30/#thematic-breaks
func parseThematicBreak(line []byte) (end int) {
	const chars = "-_*"
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

// isEndEscaped reports whether s ends with an odd number of backslashes.
func isEndEscaped(s []byte) bool {
	n := 0
	for ; n < len(s); n++ {
		if s[len(s)-n-1] != '\\' {
			break
		}
	}
	return n%2 == 1
}
