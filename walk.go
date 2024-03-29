// Copyright 2024 Ross Light
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

// A Cursor describes a [Node] encountered during [Walk].
type Cursor struct {
	node   Node
	parent Node
	block  *Block
	index  int
}

// Node returns the current [Node].
func (c *Cursor) Node() Node {
	return c.node
}

// Parent returns the parent of the current [Node]
// (as returned by [*Cursor.Node]).
func (c *Cursor) Parent() Node {
	return c.parent
}

// ParentBlock returns the nearest [Block] ancestor of the current [Node].
func (c *Cursor) ParentBlock() *Block {
	return c.block
}

// Index returns the index >= 0 of the current [Node]
// in the list of children that contains it,
// or a value < 0 if the current [Node] does not have a parent.
func (c *Cursor) Index() int {
	return c.index
}

// WalkOptions is the set of parameters to [Walk].
type WalkOptions struct {
	// If Pre is not nil, it is called for each node before the node's children are traversed (pre-order).
	// If Pre returns false, no children are traversed, and Post is not called for that node.
	Pre func(c *Cursor) bool
	// If Post is not nil, it is called for each node after the node's children are traversed (post-order).
	// If Post returns false, traversal is terminated and Walk returns immediately.
	Post func(c *Cursor) bool

	// If ChildCount is not nil, it will be used instead of [Node.ChildCount].
	ChildCount func(Node) int
	// If Child is not nil, it will be used instead of [Node.Child].
	Child func(Node, int) Node
}

// Walk traverses a [Node] recursively, starting with root,
// and calling [WalkOptions.Pre] and [WalkOptions.Post].
func Walk(root Node, opts *WalkOptions) {
	type walkFrame struct {
		Cursor
		post bool
	}

	childCount := Node.ChildCount
	if opts.ChildCount != nil {
		childCount = opts.ChildCount
	}
	getChild := Node.Child
	if opts.Child != nil {
		getChild = opts.Child
	}

	stack := []walkFrame{{Cursor: Cursor{node: root, index: -1}}}
	cursor := new(Cursor)
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if curr.post {
			if opts.Post != nil {
				*cursor = curr.Cursor
				if !opts.Post(cursor) {
					break
				}
			}
			continue
		}

		if opts.Pre != nil {
			*cursor = curr.Cursor
			if !opts.Pre(cursor) {
				continue
			}
		}
		curr.post = true
		stack = append(stack, curr)
		for i := childCount(curr.node) - 1; i >= 0; i-- {
			currBlock := curr.block
			if b := curr.node.Block(); b != nil {
				currBlock = b
			}
			stack = append(stack, walkFrame{
				Cursor: Cursor{
					parent: curr.node,
					node:   getChild(curr.node, i),
					block:  currBlock,
					index:  i,
				},
			})
		}
	}
}
