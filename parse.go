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

// Package commonmark provides a [CommonMark] parser.
//
// [CommonMark]: https://commonmark.org/
package commonmark

import (
	"bytes"
	"fmt"
	"io"
)

// tabStopSize is the multiple of columns that a [tab] advances to.
//
// [tab]: https://spec.commonmark.org/0.30/#tabs
const tabStopSize = 4

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
	bp := &blockParser{
		root:    root,
		line:    line,
		opening: true,
	}
	openNewBlocks(bp, true)
	if !root.isOpen() {
		// Single-line block.
		root.Source = p.consume()
		return root, nil
	}
	addLineText(bp)

	// Parse subsequent lines.
	for {
		bp.lineStart = p.parsePos
		bp.line = p.readline()
		bp.i = 0
		bp.tabpos = 0
		bp.opening = false

		allMatched := descendOpenBlocks(bp)

		bp.opening = true
		openNewBlocks(bp, allMatched)
		if bp.container == nil {
			if !isBlankLine(bp.Bytes()) {
				// If there's remaining text on the line,
				// then rewind to the beginning of the line.
				p.parsePos = bp.lineStart
			}
			root.Source = p.consume()
			return root, nil
		}
		addLineText(bp)
	}
}

// descendOpenBlocks iterates through the open blocks,
// starting at the top-level block,
// and descending through last children down to the last open block.
// It sets p.container to the last matched block
// or nil if not even the top-level block could be matched.
//
// This corresponds to the first step of [Phase 1]
// in the CommonMark recommended parsing strategy.
//
// [Phase 1]: https://spec.commonmark.org/0.30/#phase-1-block-structure
func descendOpenBlocks(p *blockParser) (allMatched bool) {
	p.container = nil
	child := &p.root.Block
	for {
		rule := blocks[child.Kind()]
		if rule.match == nil {
			return false
		}
		switch rule.match(p) {
		case noMatch:
			return false
		case matchedEntireLine:
			p.container = child
			return false
		}

		p.container = child
		child = child.lastChild().Block()
		if !child.isOpen() {
			return true
		}
	}
}

// openNewBlocks looks for new block starts,
// closing any blocks unmatched in step 1
// before creating new blocks as descendants of the last matched container block.
// openNewBlocks sets p.container to the deepest open block.
//
// This corresponds to the second step of [Phase 1]
// in the CommonMark recommended parsing strategy.
//
// [Phase 1]: https://spec.commonmark.org/0.30/#phase-1-block-structure
func openNewBlocks(p *blockParser, allMatched bool) {
	if len(p.Bytes()) == 0 {
		// Special case: EOF. Close the root block.
		p.root.close(p.lineStart)
		p.container = nil
		return
	}

	for p.root.isOpen() &&
		(p.ContainerKind() == ParagraphKind || !blocks[p.ContainerKind()].acceptsLines) {
		found := false
	startsLoop:
		for _, startFunc := range blockStarts {
			switch startFunc(p) {
			case noMatch:
			case matched:
				found = true
				break startsLoop
			case matchedEntireLine:
				return
			}
		}
		if !found {
			// Hit the text.
			break
		}
	}
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
		parentKind := documentKind
		if parent != nil {
			parentKind = parent.kind
		}
		if rule := blocks[parentKind]; rule.canContain != nil && rule.canContain(kind) {
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

func addLineText(p *blockParser) {
	if blocks[p.ContainerKind()].acceptsLines {
		if indent := p.Indent(); indent > 0 {
			start := p.lineStart + p.i
			p.ConsumeIndent(indent)
			p.container.children = append(p.container.children, (&Inline{
				kind:   IndentKind,
				indent: indent,
				start:  start,
				end:    p.lineStart + p.i,
			}).AsNode())
		}
		p.container.children = append(p.container.children, (&Inline{
			kind:  UnparsedKind,
			start: p.lineStart + p.i,
			end:   p.lineStart + len(p.line),
		}).AsNode())
	} else if !isBlankLine(p.Bytes()) {
		// Create paragraph container for line.
		_, para := appendNewBlock(p.root, p.container, ParagraphKind, p.lineStart, p.lineStart+p.i)
		p.ConsumeIndent(p.Indent())
		para.children = append(para.children, (&Inline{
			kind:  UnparsedKind,
			start: p.lineStart + p.i,
			end:   p.lineStart + len(p.line),
		}).AsNode())
	}
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

func trimIndent(line []byte) []byte {
	return bytes.TrimLeft(line, " \t")
}

func isBlankLine(line []byte) bool {
	for _, b := range line {
		if !(b == '\r' || b == '\n' || b == ' ' || b == '\t') {
			return false
		}
	}
	return true
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
