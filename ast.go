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

// Inline represents Markdown content elements like text, links, or emphasis.
type Inline struct {
	kind     InlineKind
	start    int
	end      int
	indent   int
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

// IndentWidth returns the number of spaces the [IndentKind] span represents.
func (inline *Inline) IndentWidth() int {
	if inline == nil {
		return 0
	}
	return inline.indent
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
	IndentKind
	ListMarkerKind
	InfoStringKind

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
