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
	"bytes"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"zombiezen.com/go/commonmark/internal/normhtml"
)

func TestNullReplacementInReference(t *testing.T) {
	const input = "[foo][foo\x00bar]\n" +
		"\n" +
		"[foo\ufffdbar]: https://www.example.com/"
	const wantHTML = `<p><a href="https://www.example.com/">foo</a></p>`

	blocks, refMap := Parse([]byte(input))
	buf := new(bytes.Buffer)
	if err := RenderHTML(buf, blocks, refMap); err != nil {
		t.Error("RenderHTML:", err)
	}
	got := string(normhtml.NormalizeHTML(buf.Bytes()))
	want := string(normhtml.NormalizeHTML([]byte(wantHTML)))
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", input, diff)
	}
}

func TestEmphasisSpan(t *testing.T) {
	const input = "oh *hello* world"

	blocks, _ := Parse([]byte(input))
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d; want 1", len(blocks))
	}
	if got, want := blocks[0].Kind(), ParagraphKind; got != want {
		t.Errorf("blocks[0].Kind() = %v; want %v", got, want)
	}

	if got, want := blocks[0].ChildCount(), 3; got != want {
		t.Fatalf("blocks[0].ChildCount() = %d; want %d", got, want)
	}
	if got, want := blocks[0].Child(0).Span(), (Span{Start: 0, End: len("oh ")}); got != want {
		t.Errorf("blocks[0].Child(0).Span() = %v; want %v", got, want)
	}
	if got, want := blocks[0].Child(1).Span(), (Span{Start: len("oh "), End: len("oh *hello*")}); got != want {
		t.Errorf("blocks[0].Child(1).Span() = %v; want %v", got, want)
	}
	if got, want := blocks[0].Child(1).Inline().Kind(), EmphasisKind; got != want {
		t.Errorf("blocks[0].Child(1).Inline().Kind() = %v; want %v", got, want)
	}
	if got, want := blocks[0].Child(2).Span(), (Span{Start: len("oh *hello*"), End: len("oh *hello* world")}); got != want {
		t.Errorf("blocks[0].Child(2).Span() = %v; want %v", got, want)
	}
}

func TestLinkSpan(t *testing.T) {
	const (
		prefix          = "oh "
		paragraphSuffix = " world"
		finalSuffix     = "\n\n[hello]: /foo"
	)

	tests := []struct {
		name string
		link string
	}{
		{
			name: "Inline",
			link: "[hello](/foo)",
		},
		{
			name: "FullReference",
			link: "[hello][hello]",
		},
		{
			name: "CollapsedReference",
			link: "[hello][]",
		},
		{
			name: "ShortcutReference",
			link: "[hello]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input []byte
			input = append(input, prefix...)
			input = append(input, test.link...)
			input = append(input, paragraphSuffix...)
			input = append(input, finalSuffix...)

			blocks, _ := Parse(input)
			if len(blocks) != 2 {
				t.Fatalf("len(blocks) = %d; want 2", len(blocks))
			}
			if got, want := blocks[0].Kind(), ParagraphKind; got != want {
				t.Errorf("blocks[0].Kind() = %v; want %v", got, want)
			}
			if got, want := blocks[1].Kind(), LinkReferenceDefinitionKind; got != want {
				t.Errorf("blocks[1].Kind() = %v; want %v", got, want)
			}

			if got, want := blocks[0].ChildCount(), 3; got != want {
				t.Fatalf("blocks[0].ChildCount() = %d; want %d", got, want)
			}
			if got, want := blocks[0].Child(0).Span(), (Span{Start: 0, End: len(prefix)}); got != want {
				t.Errorf("blocks[0].Child(0).Span() = %v; want %v", got, want)
			}
			if got, want := blocks[0].Child(1).Span(), (Span{Start: len(prefix), End: len(prefix) + len(test.link)}); got != want {
				t.Errorf("blocks[0].Child(1).Span() = %v; want %v", got, want)
			}
			if got, want := blocks[0].Child(1).Inline().Kind(), LinkKind; got != want {
				t.Errorf("blocks[0].Child(1).Inline().Kind() = %v; want %v", got, want)
			}
			if got, want := blocks[0].Child(2).Span(), (Span{Start: len(prefix) + len(test.link), End: len(prefix+paragraphSuffix) + len(test.link)}); got != want {
				t.Errorf("blocks[0].Child(2).Span() = %v; want %v", got, want)
			}
		})
	}
}

func TestDelimiterFlags(t *testing.T) {
	tests := []struct {
		prefix string
		run    string
		suffix string
		want   uint8
	}{
		// Official examples for left-flanking and right-flanking:
		{"", "***", "abc", openerFlag},
		{"  ", "_", "abc", openerFlag},
		{"", "**", `"abc"`, openerFlag},
		{" ", "_", `"abc"`, openerFlag},
		{" abc", "***", "", closerFlag},
		{" abc", "_", "", closerFlag},
		{`"abc"`, "**", "", closerFlag},
		{`"abc"`, "_", "", closerFlag},
		{" abc", "***", "def", openerFlag | closerFlag},
		{`"abc"`, "_", `"def"`, openerFlag | closerFlag},
		{"abc ", "***", " def", 0},
		{"a ", "_", " b", 0},

		// Extra examples to demonstrate
		// https://spec.commonmark.org/0.30/#can-open-emphasis
		// and
		// https://spec.commonmark.org/0.30/#can-close-emphasis.
		{"aa", "_", `"bb"`, closerFlag},
		{`"bb"`, "_", "cc", openerFlag},
		{"foo-", "_", "(bar)", openerFlag | closerFlag},
		{"(bar)", "_", "", closerFlag},
		{"abc", "_", "def", 0},
	}
	for _, test := range tests {
		source := test.prefix + test.run + test.suffix
		span := Span{
			Start: len(test.prefix),
			End:   len(test.prefix) + len(test.run),
		}
		got := emphasisFlags([]byte(source), span)
		if got != test.want {
			t.Errorf("delimiterFlags(%q, %#v) = %#03b; want %#03b", source, span, got, test.want)
		}
	}
}

func FuzzInlineParsing(f *testing.F) {
	for _, test := range loadTestSuite(f) {
		f.Add(test.Markdown)
	}

	f.Fuzz(func(t *testing.T, markdown string) {
		if !utf8.ValidString(markdown) {
			t.Skip("Invalid UTF-8")
		}
		p := NewBlockParser(strings.NewReader(markdown))
		var blocks []*RootBlock
		refMap := make(ReferenceMap)
		for i := 0; ; i++ {
			block, err := p.NextBlock()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}

			blocks = append(blocks, block)
			refMap.Extract(block.Source, block.AsNode())
		}

		inlineParser := &InlineParser{
			ReferenceMatcher: refMap,
		}
		for _, block := range blocks {
			inlineParser.Rewrite(block)
			verifySpansDontExceedParents(t, block.AsNode(), Span{
				Start: 0,
				End:   len(block.Source),
			})
		}
		if t.Failed() {
			t.Logf("Input:\n%s", markdown)
		}
	})
}
