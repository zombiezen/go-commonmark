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

package commonmark_test

import (
	"io"
	"os"
	"strings"

	"zombiezen.com/go/commonmark"
)

func Example() {
	// Convert CommonMark to a parse tree and any link references.
	blocks, refMap := commonmark.Parse([]byte("Hello, **World**!\n"))
	// Render parse tree to HTML.
	commonmark.RenderHTML(os.Stdout, blocks, refMap)
	// Output:
	// <p>Hello, <strong>World</strong>!</p>
}

func ExampleBlockParser() {
	input := strings.NewReader(
		"Hello, [World][]!\n" +
			"\n" +
			"[World]: https://www.example.com/\n",
	)

	// Parse document into blocks (e.g. paragraphs, lists, etc.)
	// and collect link reference definitions.
	parser := commonmark.NewBlockParser(input)
	var blocks []*commonmark.RootBlock
	refMap := make(commonmark.ReferenceMap)
	for {
		block, err := parser.NextBlock()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Not expecting an error from a string.
			panic(err)
		}

		// Add block to list.
		blocks = append(blocks, block)
		// Add any link reference definitions to map.
		refMap.Extract(block.Source, block.AsNode())
	}

	// Finish parsing inside blocks.
	inlineParser := &commonmark.InlineParser{
		ReferenceMatcher: refMap,
	}
	for _, block := range blocks {
		inlineParser.Rewrite(block)
	}

	// Render blocks as HTML.
	commonmark.RenderHTML(os.Stdout, blocks, refMap)
	// Output:
	// <p>Hello, <a href="https://www.example.com/">World</a>!</p>
}
