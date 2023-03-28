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

// Inline represents Markdown content elements like text, links, or emphasis.
type Inline struct {
	kind     InlineKind
	start    int
	end      int
	indent   int
	children []*Inline
}

// Kind returns the type of inline node
// or zero if the node is nil.
func (inline *Inline) Kind() InlineKind {
	if inline == nil {
		return 0
	}
	return InlineKind(inline.kind)
}

// Start returns the offset in [RootBlock.Source]
// where the inline node starts,
// or -1 if the node is nil.
func (inline *Inline) Start() int {
	if inline == nil {
		return -1
	}
	return inline.start
}

// End returns the offset in [RootBlock.Source]
// where the inline node ends (exclusive),
// or -1 if the node is nil.
func (inline *Inline) End() int {
	if inline == nil {
		return -1
	}
	return inline.end
}

// IndentWidth returns the number of spaces the [IndentKind] span represents,
// or zero if the node is nil or of a different type.
func (inline *Inline) IndentWidth() int {
	if inline == nil {
		return 0
	}
	return inline.indent
}

// Children returns children of the node.
// Calling Children on nil returns a nil slice.
func (inline *Inline) Children() []*Inline {
	if inline == nil {
		return nil
	}
	return inline.children
}

// InlineKind is an enumeration of values returned by [*Inline.Kind].
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
