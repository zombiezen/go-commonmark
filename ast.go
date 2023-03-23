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

func (kind BlockKind) acceptsLines() bool {
	return kind == ParagraphKind ||
		kind == IndentedCodeBlockKind ||
		kind == FencedCodeBlockKind
}

func (kind BlockKind) canContain(childKind BlockKind) bool {
	switch kind {
	case ListKind:
		return childKind == ListItemKind
	case ListItemKind, BlockQuoteKind, documentKind:
		return childKind != ListItemKind
	default:
		return false
	}
}

// Inline represents Markdown content elements like text, links, or emphasis.
type Inline struct {
	kind     InlineKind
	start    int
	end      int
	children []*Inline
}

func (inline *Inline) Kind() InlineKind {
	if inline == nil {
		return 0
	}
	return InlineKind(inline.kind)
}

func (inline *Inline) Start() int {
	if inline == nil {
		return -1
	}
	return inline.start
}

func (inline *Inline) End() int {
	if inline == nil {
		return -1
	}
	return inline.end
}

func (inline *Inline) Children() []*Inline {
	if inline == nil {
		return nil
	}
	return inline.children
}

func (inline *Inline) AsNode() Node {
	if inline == nil {
		return Node{}
	}
	return Node{
		typ: nodeTypeInline,
		ptr: unsafe.Pointer(inline),
	}
}

type InlineKind uint16

const (
	TextKind InlineKind = 1 + iota
	SoftLineBreakKind
	HardLineBreakKind
	CodeIndentKind

	// UnparsedKind is used for inline text that has not been tokenized.
	UnparsedKind
)

const (
	nodeTypeBlock = 1 + iota
	nodeTypeInline
)

// Node is a pointer to a [Block] or an [Inline].
type Node struct {
	ptr unsafe.Pointer
	typ uint8
}

// Block returns the referenced block
// or nil if the pointer does not reference a block.
func (n Node) Block() *Block {
	if n.typ != nodeTypeBlock {
		return nil
	}
	return (*Block)(n.ptr)
}

// Inline returns the referenced block
// or nil if the pointer does not reference an inline.
func (n Node) Inline() *Inline {
	if n.typ != nodeTypeInline {
		return nil
	}
	return (*Inline)(n.ptr)
}
