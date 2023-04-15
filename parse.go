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
	"unicode"
)

// tabStopSize is the multiple of columns that a [tab] advances to.
//
// [tab]: https://spec.commonmark.org/0.30/#tabs
const tabStopSize = 4

// A BlockParser splits a CommonMark document into blocks.
type BlockParser struct {
	buf    []byte // current block being parsed
	offset int64  // offset from beginning of stream to beginning of buf
	lineno int    // line number of beginning of buf
	i      int    // parse position within buf

	r   io.Reader
	err error // non-nil indicates there is no more data after end of buf

	blocks []*Block
}

// NewBlockParser returns a block parser that reads from r.
//
// Block parsers maintain their own buffering and may read data from r
// beyond the blocks requested.
func NewBlockParser(r io.Reader) *BlockParser {
	return &BlockParser{r: r}
}

// Parse parses an in-memory CommonMark document and returns its blocks.
// As long as source does not contain NUL bytes,
// the blocks will use the original byte slice as their source.
func Parse(source []byte) ([]*RootBlock, ReferenceMap) {
	if bytes.IndexByte(source, 0) >= 0 {
		// Contains one or more NUL bytes.
		// Replace with Unicode replacement character.
		source = bytes.ReplaceAll(source, []byte{0}, []byte("\ufffd"))
	}
	p := &BlockParser{
		buf: source,
		err: io.EOF,
	}
	var blocks []*RootBlock
	refMap := make(ReferenceMap)
	for {
		block, err := p.NextBlock()
		if err == io.EOF {
			inlineParser := &InlineParser{
				ReferenceMatcher: refMap,
			}
			for _, block := range blocks {
				inlineParser.Rewrite(block)
			}
			return blocks, refMap
		}
		if err != nil {
			panic(err)
		}
		blocks = append(blocks, block)
		refMap.Extract(block.Source, block.AsNode())
	}
}

// NextBlock reads the next top-level block in the document,
// returning the first error encountered.
// Blocks returned by NextBlock will typically contain [UnparsedKind] nodes for any text:
// use [*InlineParser.Rewrite] to complete parsing.
func (p *BlockParser) NextBlock() (*RootBlock, error) {
	// If we have any leftover closed blocks from previous calls,
	// return those first.
	if next := p.makeRoot(p.blocks); next != nil {
		return next, nil
	}

	lineStart := 0
	if len(p.blocks) > 0 {
		lineStart = p.i
		p.readline()
	} else {
		// If we don't have any pending blocks,
		// then we either just started or we previously hit a blank line.
		p.offset += int64(p.i)
		p.lineno += lineCount(p.buf[:p.i])
		p.buf = p.buf[p.i:]
		p.i = 0

		// Keep going until we encounter a non-blank line.
		for {
			if !p.readline() {
				return nil, p.err
			}
			if !isBlankLine(p.buf[:p.i]) {
				break
			}
			p.offset += int64(p.i)
			p.lineno++
			p.buf = p.buf[p.i:]
			p.i = 0
		}
	}

	// Parse lines.
	lp := newLineParser(p.blocks, lineStart, p.buf[:p.i:p.i])
	for {
		allMatched := descendOpenBlocks(lp)
		hasText := false
		if lp.state != stateDescendTerminated {
			hasText = openNewBlocks(lp, allMatched)
		}
		if hasText {
			addLineText(lp)
		}
		if next := p.makeRoot(lp.root.blockChildren); next != nil {
			return next, nil
		}

		lineStart := p.i
		p.readline()
		lp.reset(lineStart, p.buf[:p.i:p.i])
	}
}

func (p *BlockParser) makeRoot(docChildren []*Block) *RootBlock {
	if len(docChildren) == 0 || docChildren[0].isOpen() {
		return nil
	}
	n := docChildren[0].Span().End
	block := &RootBlock{
		Source:      p.buf[:n:n],
		StartLine:   p.lineno,
		StartOffset: p.offset,
		Block:       *docChildren[0],
	}

	// Store any remaining children for later use, updating offsets.
	p.blocks = docChildren[1:]
	for _, b := range p.blocks {
		offsetTree(b.AsNode(), -n)
	}

	// Advance parser state.
	p.offset += int64(n)
	p.lineno += lineCount(p.buf[:n])
	p.buf = p.buf[n:]
	p.i -= n

	return block
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
func descendOpenBlocks(p *lineParser) (allMatched bool) {
	parent := &p.root
	p.container = parent.lastChild().Block()
	for p.container.isOpen() {
		rule := blocks[p.ContainerKind()]
		if rule.match == nil {
			p.container = parent
			return false
		}
		p.state = stateDescending
		ok := rule.match(p)
		if p.state == stateDescendTerminated {
			p.container.close(p.source, parent, p.lineStart+p.i)
			p.container = parent
			return true
		}
		if !ok {
			p.container = parent
			return false
		}

		parent = p.container
		p.container = parent.lastChild().Block()
	}
	p.container = parent
	return true
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
func openNewBlocks(p *lineParser, allMatched bool) (hasText bool) {
	if len(p.line) == 0 {
		// Special case: EOF. Close the document block.
		p.root.close(p.source, nil, p.lineStart)
		p.container = nil
		return false
	}

	// If we didn't match everything in [descendOpenBlocks],
	// we may need to close descendants if no new blocks were created.
	// (Creating a new block automatically closes prior open children.)
	if !allMatched {
		defer func() {
			// Special case: [paragraph continuation text].
			// Rather than closing the unmatched paragraph,
			// move the container pointer to it.
			//
			// [paragraph continuation text]: https://spec.commonmark.org/0.30/#paragraph-continuation-text
			if !p.IsRestBlank() {
				if tip := findTip(&p.root); tip.Kind() == ParagraphKind {
					p.container = tip
					return
				}
			}

			p.container.lastChild().Block().close(p.source, p.container, p.lineStart)
		}()
	}

openingLoop:
	for p.ContainerKind() == ParagraphKind || !blocks[p.ContainerKind()].acceptsLines {
		for _, startFunc := range blockStarts {
			p.state = stateOpening
			startFunc(p)
			switch p.state {
			case stateOpenMatched:
				continue openingLoop
			case stateLineConsumed:
				return false
			}
		}
		// Hit the text.
		return true
	}
	return true
}

func addLineText(p *lineParser) {
	// Record whether a block ends in a blank line
	// for the purpose of checking for list looseness.
	isBlank := p.IsRestBlank()
	if lastChild := p.container.lastChild().Block(); lastChild != nil && isBlank {
		lastChild.lastLineBlank = true
	}
	lastLineBlank := isBlank && !(p.ContainerKind() == BlockQuoteKind ||
		p.ContainerKind() == FencedCodeBlockKind ||
		(p.ContainerKind() == ListItemKind && p.container.ChildCount() == 1 && p.container.Span().Start >= p.lineStart))
	// Propagate lastLineBlank up through parents:
	for c := p.container; c != nil; c = findParent(&p.root, c) {
		c.lastLineBlank = lastLineBlank
	}

	switch {
	case blocks[p.ContainerKind()].acceptsLines:
		if indent := p.Indent(); indent > 0 {
			start := p.lineStart + p.i
			p.ConsumeIndent(indent)
			p.container.inlineChildren = append(p.container.inlineChildren, &Inline{
				kind:   IndentKind,
				indent: indent,
				span: Span{
					Start: start,
					End:   p.lineStart + p.i,
				},
			})
		}
	case !isBlank:
		// Create paragraph container for line.
		p.OpenBlock(ParagraphKind)
		p.ConsumeIndent(p.Indent())
	default:
		return
	}

	inlineKind := UnparsedKind
	switch {
	case p.ContainerKind().IsCode():
		inlineKind = TextKind
	case p.ContainerKind() == HTMLBlockKind:
		inlineKind = RawHTMLKind
	}
	p.container.inlineChildren = append(p.container.inlineChildren, &Inline{
		kind: inlineKind,
		span: Span{
			Start: p.lineStart + p.i,
			End:   p.lineStart + len(p.line),
		},
	})
}

func findParent(root *Block, b *Block) *Block {
	for parent, curr := (*Block)(nil), root; ; {
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

// findTip finds the deepest open descendant of b.
func findTip(b *Block) *Block {
	var parent *Block
	for b.isOpen() {
		parent, b = b, b.lastChild().Block()
	}
	return parent
}

// offsetTree adds n to every offset in the tree.
func offsetTree(node Node, n int) {
	stack := []Node{node}
	for len(stack) > 0 {
		curr := stack[0]
		stack = stack[1:]
		switch {
		case curr.Block() != nil:
			block := curr.Block()
			block.span.Start += n
			if block.span.End >= 0 {
				block.span.End += n
			}
			for i := block.ChildCount() - 1; i >= 0; i-- {
				stack = append(stack, block.Child(i))
			}
		case curr.Inline() != nil:
			inline := curr.Inline()
			inline.span.Start += n
			if inline.span.End >= 0 {
				inline.span.End += n
			}
			for i := inline.ChildCount() - 1; i >= 0; i-- {
				stack = append(stack, inline.Child(i).AsNode())
			}
		}
	}
}

// readline advances p.i to the end of the next line of input,
// returning false if it has reached the end of input.
// readline saves the line into p.buf, growing it as necessary.
func (p *BlockParser) readline() bool {
	const (
		chunkSize    = 8 * 1024
		maxBlockSize = 1024 * 1024
	)

	eolEnd := -1
	for {
		// Check if we have a line ending available.
		if i := bytes.IndexAny(p.buf[p.i:], "\r\n"); i >= 0 {
			eolStart := p.i + i
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
			p.buf = p.buf[:p.i]
			p.err = fmt.Errorf("line %d: block too large", p.lineno)
			return false
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

	ok := p.i < eolEnd
	p.i = eolEnd
	return ok
}

func lineCount(text []byte) int {
	count := 0
	for i, b := range text {
		switch b {
		case '\n':
			count++
		case '\r':
			if i+1 >= len(text) || text[i+1] != '\n' {
				count++
			}
		}
	}
	return count
}

// columnWidth returns the width of the given text in columns
// given the 0-based column starting position.
func columnWidth(start int, b []byte) int {
	end := start
	for _, bi := range b {
		switch {
		case bi == '\t':
			// Assumes tabStopSize is a power-of-two.
			end = (end + tabStopSize) &^ (tabStopSize - 1)
		case bi&0x80 == 0:
			// End of code point or ASCII character.
			end++
		}
	}
	return end - start
}

// indentLength returns the number of space or tab bytes
// at the beginning of the slice.
func indentLength(line []byte) int {
	for i, b := range line {
		if b != ' ' && b != '\t' {
			return i
		}
	}
	return len(line)
}

// Span is a contiguous region of a document
// reference in a [RootBlock].
type Span struct {
	// Start is the index of the first byte of the span,
	// relative to the beginning of the [RootBlock].
	Start int
	// End is the end index of the span (exclusive),
	// relative to the beginning of the [RootBlock].
	End int
}

// NullSpan returns an invalid span.
func NullSpan() Span {
	return Span{-1, -1}
}

func spanSlice(b []byte, span Span) []byte {
	return b[span.Start:span.End]
}

// Len returns the length of the span
// or zero if the span is invalid.
func (span Span) Len() int {
	if !span.IsValid() {
		return 0
	}
	return span.End - span.Start
}

// Intersect returns the intersection of two spans
// or an invalid span if none exists.
func (span Span) Intersect(span2 Span) Span {
	if !span.IsValid() || !span2.IsValid() || span.Start >= span2.End || span.End <= span2.Start {
		return NullSpan()
	}
	result := span
	if span2.Start > span.Start {
		result.Start = span2.Start
	}
	if span2.End < span.End {
		result.End = span2.End
	}
	return result
}

// IsValid reports whether the span is valid.
func (span Span) IsValid() bool {
	return span.Start >= 0 && span.End >= 0 && span.Start <= span.End
}

// String formats the span indices as a mathematical range like "[12,34)".
func (span Span) String() string {
	return fmt.Sprintf("[%d,%d)", span.Start, span.End)
}

func isBlankLine(line []byte) bool {
	for _, b := range line {
		if !isSpaceTabOrLineEnding(b) {
			return false
		}
	}
	return true
}

func hasTabOrSpacePrefixOrEOL(line []byte) bool {
	return len(line) == 0 || isSpaceTabOrLineEnding(line[0])
}

// isSpaceTabOrLineEnding reports whether c is a space, tab, or line ending character.
func isSpaceTabOrLineEnding(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isASCIILetter(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z'
}

func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isASCIIPunctuation(c byte) bool {
	return '!' <= c && c <= '/' ||
		':' <= c && c <= '@' ||
		'[' <= c && c <= '`' ||
		'{' <= c && c <= '~'
}

func isASCIIControl(c byte) bool {
	return c <= 0x1f || c == 0x7f
}

// isUnicodeWhitespace reports whether the code point is a [Unicode whitespace character].
//
// [Unicode whitespace character]: https://spec.commonmark.org/0.30/#unicode-whitespace-character
func isUnicodeWhitespace(c rune) bool {
	return (c <= 0x7f && isSpaceTabOrLineEnding(byte(c))) || unicode.Is(unicode.Zs, c)
}

// isUnicodePunctuation reports whether the code point is a [Unicode punctuation character].
//
// [Unicode punctuation character]: https://spec.commonmark.org/0.30/#unicode-punctuation-character
func isUnicodePunctuation(c rune) bool {
	if c < 0x80 {
		return isASCIIPunctuation(byte(c))
	}
	return unicode.In(c, unicode.Pc, unicode.Pd, unicode.Pe, unicode.Pf, unicode.Pi, unicode.Po, unicode.Ps)
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

func hasBytePrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i, bb := range b[:len(prefix)] {
		if prefix[i] != bb {
			return false
		}
	}
	return true
}

func contains(b []byte, search string) bool {
	for i := 0; i < len(b)-len(search); i++ {
		if hasBytePrefix(b[i:], search) {
			return true
		}
	}
	return false
}
