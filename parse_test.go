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

import (
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestInsecureCharacters(t *testing.T) {
	const input = "Hello,\x00World"
	const want = "Hello,\ufffdWorld"

	type testCase struct {
		name   string
		blocks []*RootBlock
		err    error
	}
	var tests []testCase

	memBlocks, _ := Parse([]byte(input))
	tests = append(tests, testCase{
		name:   "Parse",
		blocks: memBlocks,
	})

	blockParserTest := testCase{name: "BlockParser"}
	for p := NewBlockParser(strings.NewReader(input)); ; {
		block, err := p.NextBlock()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				blockParserTest.err = err
			}
			break
		}
		new(InlineParser).Rewrite(block)
		blockParserTest.blocks = append(blockParserTest.blocks, block)
	}
	tests = append(tests, blockParserTest)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err != nil {
				t.Error("During read:", test.err)
			}

			if len(test.blocks) != 1 {
				t.Fatalf("len(blocks) = %d; want 1", len(test.blocks))
			}
			if got := test.blocks[0].Kind(); got != ParagraphKind {
				t.Fatalf("blocks[0].Kind() = %v; want %v", got, ParagraphKind)
			}
			if got := test.blocks[0].ChildCount(); got != 1 {
				t.Fatalf("blocks[0].ChildCount() = %d; want 1", got)
			}
			inline := test.blocks[0].Child(0).Inline()
			if got := inline.Kind(); got != TextKind {
				t.Fatalf("blocks[0].Child(0).Inline().Kind() = %v; want %v", got, TextKind)
			}
			got := inline.Text(test.blocks[0].Source)
			if got != want {
				t.Errorf("blocks[0].Child(0).Inline().Text(...) = %q; want %q", got, want)
			}
		})
	}
}

func FuzzBlockParsing(f *testing.F) {
	for _, test := range loadTestSuite(f) {
		f.Add(test.Markdown)
	}

	f.Fuzz(func(t *testing.T, markdown string) {
		if !utf8.ValidString(markdown) {
			t.Skip("Invalid UTF-8")
		}
		p := NewBlockParser(strings.NewReader(markdown))
		for i := 0; ; i++ {
			block, err := p.NextBlock()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}

			// Verify position information.
			if block.StartOffset > int64(len(markdown)) {
				t.Errorf("blocks[%d]: StartOffset = %d; want <%d", i, block.StartOffset, len(markdown))
				continue
			}
			if block.StartOffset+int64(len(block.Source)) > int64(len(markdown)) {
				t.Errorf("blocks[%d]: StartOffset = %d, len(Source) = %d, StartOffset+len(Source) = %d; want <%d", i, block.StartOffset, len(block.Source), block.StartOffset+int64(len(block.Source)), len(markdown))
				continue
			}
			if want := markdown[block.StartOffset : int(block.StartOffset)+len(block.Source)]; string(block.Source) != want {
				t.Errorf("blocks[%d]: for StartOffset=%d, Source = %q; want %q", i, block.StartOffset, block.Source, want)
			}
			if want := lineCount([]byte(markdown[:block.StartOffset])) + 1; block.StartLine != want {
				t.Errorf("blocks[%d]: for StartOffset=%d, StartLine = %d; want %d", i, block.StartOffset, block.StartLine, want)
			}

			// Verify span content.
			verifySpansDontExceedParents(t, block.AsNode(), Span{
				Start: 0,
				End:   len(block.Source),
			})
		}
	})
}

func verifySpansDontExceedParents(tb testing.TB, n Node, parentSpan Span) {
	tb.Helper()

	if b := n.Block(); b != nil {
		ns := b.Span()
		if ns.Start < parentSpan.Start || ns.End > parentSpan.End {
			tb.Errorf("%v node span %v exceeds parent span %v", b.Kind(), ns, parentSpan)
		}
		for i := 0; i < b.ChildCount(); i++ {
			verifySpansDontExceedParents(tb, b.Child(i), b.Span())
		}
	}

	if inline := n.Inline(); inline != nil {
		ns := inline.Span()
		if ns.Start < parentSpan.Start || ns.End > parentSpan.End {
			tb.Errorf("%v node span %v exceeds parent span %v", inline.Kind(), ns, parentSpan)
		}
		for i := 0; i < inline.ChildCount(); i++ {
			verifySpansDontExceedParents(tb, inline.Child(i).AsNode(), inline.Span())
		}
	}
}
