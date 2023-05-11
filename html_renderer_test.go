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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"zombiezen.com/go/commonmark/internal/normhtml"
)

func TestSoftBreakBehavior(t *testing.T) {
	tests := []struct {
		name     string
		behavior SoftBreakBehavior
		input    string
		want     string
	}{
		{
			name:     "PreserveLF",
			behavior: SoftBreakPreserve,
			input:    "Hello\nWorld!",
			want:     "<p>Hello\nWorld!</p>",
		},
		{
			name:     "PreserveCRLF",
			behavior: SoftBreakPreserve,
			input:    "Hello\r\nWorld!",
			want:     "<p>Hello\r\nWorld!</p>",
		},
		{
			name:     "Space",
			behavior: SoftBreakSpace,
			input:    "Hello\r\nWorld!",
			want:     "<p>Hello World!</p>",
		},
		{
			name:     "Harden",
			behavior: SoftBreakHarden,
			input:    "Hello\r\nWorld!",
			want:     "<p>Hello<br>\nWorld!</p>",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.input))
			r := &HTMLRenderer{
				ReferenceMap:      refMap,
				SoftBreakBehavior: test.behavior,
			}
			buf := new(bytes.Buffer)
			if err := r.Render(buf, blocks); err != nil {
				t.Error("Render:", err)
			}
			if got := buf.String(); got != test.want {
				t.Errorf("output = %q; want %q", got, test.want)
			}
		})
	}
}

func TestHTMLRendererIgnoreRaw(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NoRaw",
			input: "Hello World!",
			want:  "<p>Hello World!</p>",
		},
		{
			name:  "MarkdownStrong",
			input: "Hello **World**!",
			want:  "<p>Hello <strong>World</strong>!</p>",
		},
		{
			name:  "HTMLStrong",
			input: "Hello <strong>World</strong>!",
			want:  "<p>Hello World!</p>",
		},
		{
			name:  "HTMLBlock",
			input: "<table>\n<tr><td>Hello</td></tr>\n</table>",
			want:  "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.input))
			r := &HTMLRenderer{
				ReferenceMap: refMap,
				IgnoreRaw:    true,
			}
			buf := new(bytes.Buffer)
			if err := r.Render(buf, blocks); err != nil {
				t.Error("Render:", err)
			}
			if got := buf.String(); got != test.want {
				t.Errorf("output = %q; want %q", got, test.want)
			}
		})
	}
}

func TestHTMLRendererFilter(t *testing.T) {
	t.Skip("Not implemented yet.")

	tests := []struct {
		name       string
		input      string
		filterTag  func(tag []byte) bool
		skipFilter bool
		want       string
	}{
		{
			name: "GFMExample/Default",
			input: "<strong> <title> <style> <em>\n\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
			want: "<p><strong> &lt;title> &lt;style> <em></p>\n" +
				"<blockquote>\n" +
				"  &lt;xmp> is disallowed.  &lt;XMP> is also disallowed.\n" +
				"</blockquote>",
		},
		{
			name: "GFMExample/SkipFilter",
			input: "<strong> <title> <style> <em>\n\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
			skipFilter: true,
			want: "<p><strong> <title> <style> <em></p>\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
		},
		{
			name: "GFMExample/AllowAll",
			input: "<strong> <title> <style> <em>\n\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
			filterTag: func(tag []byte) bool { return false },
			want: "<p><strong> <title> <style> <em></p>\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
		},
		{
			name: "GFMExample/BlockAll",
			input: "<strong> <title> <style> <em>\n\n" +
				"<blockquote>\n" +
				"  <xmp> is disallowed.  <XMP> is also disallowed.\n" +
				"</blockquote>\n",
			filterTag: func(tag []byte) bool { return true },
			want: "&lt;p>&lt;strong> &lt;title> &lt;style> &lt;em>&lt;/p>\n" +
				"&lt;blockquote>\n" +
				"  &lt;xmp> is disallowed.  &lt;XMP> is also disallowed.\n" +
				"&lt;/blockquote>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.input))
			r := &HTMLRenderer{
				ReferenceMap: refMap,
				FilterTag:    test.filterTag,
				SkipFilter:   test.skipFilter,
			}
			buf := new(bytes.Buffer)
			if err := r.Render(buf, blocks); err != nil {
				t.Error("Render:", err)
			}
			got := normhtml.NormalizeHTML(buf.Bytes())
			want := normhtml.NormalizeHTML([]byte(test.want))
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("-want +got:\n%s", diff)
			}
		})
	}
}

func BenchmarkRenderHTML(b *testing.B) {
	b.Run("Spec", func(b *testing.B) {
		input := new(bytes.Buffer)
		testsuite := loadTestSuite(b)
		for i, test := range testsuite {
			if i > 0 {
				input.WriteString("\n\n")
			}
			input.WriteString(test.Markdown)
		}
		doc, refMap := Parse(input.Bytes())
		b.ResetTimer()
		b.SetBytes(int64(input.Len()))
		b.ReportMetric(float64(len(testsuite)), "examples/op")

		for i := 0; i < b.N; i++ {
			RenderHTML(io.Discard, doc, refMap)
		}
	})

	b.Run("Goldmark", func(b *testing.B) {
		input, err := os.ReadFile(filepath.Join("testdata", "goldmark_bench.md"))
		if err != nil {
			b.Fatal(err)
		}
		doc, refMap := Parse(input)
		b.ResetTimer()
		b.SetBytes(int64(len(input)))

		for i := 0; i < b.N; i++ {
			RenderHTML(io.Discard, doc, refMap)
		}
	})
}
