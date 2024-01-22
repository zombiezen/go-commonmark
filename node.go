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

const (
	nodeTypeBlock = 1 + iota
	nodeTypeInline
)

// Node is a pointer to a [Block] or an [Inline].
// Nodes can be compared for equality using the == operator.
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

// Inline returns the referenced inline
// or nil if the pointer does not reference an inline.
func (n Node) Inline() *Inline {
	if n.typ != nodeTypeInline {
		return nil
	}
	return (*Inline)(n.ptr)
}

// Span returns the span of the referenced node
// or an invalid span if the pointer is nil.
func (n Node) Span() Span {
	if b := n.Block(); b != nil {
		return b.Span()
	}
	if i := n.Inline(); i != nil {
		return i.Span()
	}
	return NullSpan()
}

// ChildCount returns the number of children the node has.
// Calling ChildCount on the zero value returns 0.
func (n Node) ChildCount() int {
	if b := n.Block(); b != nil {
		return b.ChildCount()
	}
	if i := n.Inline(); i != nil {
		return i.ChildCount()
	}
	return 0
}

// Child returns the i'th child of the node.
func (n Node) Child(i int) Node {
	if b := n.Block(); b != nil {
		return b.Child(i)
	}
	if in := n.Inline(); in != nil {
		return in.Child(i).AsNode()
	}
	panic("Child on nil Node")
}

// AsNode converts the inline node to a [Node] pointer.
func (inline *Inline) AsNode() Node {
	if inline == nil {
		return Node{}
	}
	return Node{
		typ: nodeTypeInline,
		ptr: unsafe.Pointer(inline),
	}
}

// AsNode converts the block node to a [Node] pointer.
func (b *Block) AsNode() Node {
	if b == nil {
		return Node{}
	}
	return Node{
		typ: nodeTypeBlock,
		ptr: unsafe.Pointer(b),
	}
}
