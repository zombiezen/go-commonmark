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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go4.org/bytereplacer"
	"golang.org/x/net/html"
)

func TestSpec(t *testing.T) {
	for _, test := range loadTestSuite(t) {
		t.Run(fmt.Sprintf("Example%d", test.Example), func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.Markdown))
			buf := new(bytes.Buffer)
			if err := RenderHTML(buf, blocks, refMap); err != nil {
				t.Error("RenderHTML:", err)
			}
			got := string(normalizeHTML(buf.Bytes()))
			want := string(normalizeHTML([]byte(test.HTML)))
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", test.Markdown, diff)
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

func FuzzCommonMarkJS(f *testing.F) {
	if testing.Short() {
		f.Skip("Skipping due to -short")
	}

	commonmarkArgs := nixShellCommand(f, "commonmark-js", "commonmark")
	for _, test := range loadTestSuite(f) {
		f.Add(test.Markdown)
	}

	f.Fuzz(func(t *testing.T, markdown string) {
		if !utf8.ValidString(markdown) {
			t.Skip("Invalid UTF-8")
		}
		blocks, refMap := Parse([]byte(markdown))
		buf := new(bytes.Buffer)
		if err := RenderHTML(buf, blocks, refMap); err != nil {
			t.Error("RenderHTML:", err)
		}
		got := string(normalizeHTML(buf.Bytes()))

		c := exec.Command(commonmarkArgs[0], commonmarkArgs[1:]...)
		c.Stdin = strings.NewReader(markdown)
		c.Stderr = os.Stderr
		rawWant, err := c.Output()
		if err != nil {
			t.Fatal(err)
		}
		want := string(normalizeHTML(rawWant))

		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", markdown, diff)
		}
	})
}

func nixShellCommand(tb testing.TB, pkg string, programName string) []string {
	tb.Helper()

	nixExe, err := exec.LookPath("nix")
	if err != nil {
		tb.Logf("Could not find Nix (falling back to using %s directly): %v", programName, err)
		exe, err := exec.LookPath(programName)
		if err != nil {
			tb.Skip(err)
		}
		return []string{exe}
	}

	return []string{
		nixExe,
		"--extra-experimental-features", "nix-command flakes",
		"shell",
		".#" + pkg,
		"--quiet",
		"--no-warn-dirty",
		"--command", programName,
	}
}

type specExample struct {
	Markdown string
	HTML     string
	Example  int
	Section  string
}

func loadTestSuite(tb testing.TB) []specExample {
	tb.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "spec-0.30.json"))
	if err != nil {
		tb.Fatal(err)
	}
	var testsuite []specExample
	if err := json.Unmarshal(data, &testsuite); err != nil {
		tb.Fatal(err)
	}
	return testsuite
}

var whitespaceRE = regexp.MustCompile(`\s+`)

var htmlEscaper = bytereplacer.New(
	"&", "&amp;",
	`'`, "&#39;", // "&#39;" is shorter than "&apos;" and apos was not in HTML until HTML5.
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&quot;",
)

func normalizeHTML(b []byte) []byte {
	type htmlAttribute struct {
		key   string
		value string
	}

	tok := html.NewTokenizer(bytes.NewReader(b))
	var output []byte
	last := html.StartTagToken
	var lastTag string
	inPre := false
	for {
		tt := tok.Next()
		switch tt {
		case html.ErrorToken:
			return output
		case html.TextToken:
			data := tok.Text()
			afterTag := last == html.EndTagToken || last == html.StartTagToken
			afterBlockTag := afterTag && isBlockTag(lastTag)
			if afterTag && lastTag == "br" {
				data = bytes.TrimLeft(data, "\n")
			}
			if !inPre {
				data = whitespaceRE.ReplaceAll(data, []byte(" "))
			}
			if afterBlockTag && !inPre {
				if last == html.StartTagToken {
					data = bytes.TrimLeftFunc(data, unicode.IsSpace)
				} else if last == html.EndTagToken {
					data = bytes.TrimSpace(data)
				}
			}
			output = append(output, htmlEscaper.Replace(bytes.Clone(data))...)
		case html.EndTagToken:
			tagBytes, _ := tok.TagName()
			tag := string(tagBytes)
			if tag == "pre" {
				inPre = false
			} else if isBlockTag(tag) {
				output = bytes.TrimRightFunc(output, unicode.IsSpace)
			}
			output = append(output, "</"...)
			output = append(output, tag...)
			output = append(output, ">"...)
			lastTag = tag
		case html.StartTagToken, html.SelfClosingTagToken:
			tagBytes, hasAttr := tok.TagName()
			tag := string(tagBytes)
			if tag == "pre" {
				inPre = true
			}
			if isBlockTag(tag) {
				output = bytes.TrimRightFunc(output, unicode.IsSpace)
			}
			output = append(output, "<"...)
			output = append(output, tag...)
			if hasAttr {
				var attrs []htmlAttribute
				for {
					k, v, more := tok.TagAttr()
					attrs = append(attrs, htmlAttribute{string(k), string(v)})
					if !more {
						break
					}
				}
				sort.Slice(attrs, func(i, j int) bool {
					return attrs[i].key < attrs[j].key
				})
				for _, attr := range attrs {
					output = append(output, " "...)
					output = append(output, attr.key...)
					if attr.value != "" {
						output = append(output, `="`...)
						output = append(output, html.EscapeString(attr.value)...)
						output = append(output, `"`...)
					}
				}
			}
			output = append(output, ">"...)
			lastTag = tag
		case html.CommentToken:
			output = append(output, tok.Raw()...)
		}

		last = tt
		if tt == html.SelfClosingTagToken {
			last = html.EndTagToken
		}
	}
}

var blockTags = map[string]struct{}{
	"article":    {},
	"header":     {},
	"aside":      {},
	"hgroup":     {},
	"blockquote": {},
	"hr":         {},
	"iframe":     {},
	"body":       {},
	"li":         {},
	"map":        {},
	"button":     {},
	"object":     {},
	"canvas":     {},
	"ol":         {},
	"caption":    {},
	"output":     {},
	"col":        {},
	"p":          {},
	"colgroup":   {},
	"pre":        {},
	"dd":         {},
	"progress":   {},
	"div":        {},
	"section":    {},
	"dl":         {},
	"table":      {},
	"td":         {},
	"dt":         {},
	"tbody":      {},
	"embed":      {},
	"textarea":   {},
	"fieldset":   {},
	"tfoot":      {},
	"figcaption": {},
	"th":         {},
	"figure":     {},
	"thead":      {},
	"footer":     {},
	"tr":         {},
	"form":       {},
	"ul":         {},
	"h1":         {},
	"h2":         {},
	"h3":         {},
	"h4":         {},
	"h5":         {},
	"h6":         {},
	"video":      {},
	"script":     {},
	"style":      {},
}

func isBlockTag(tag string) bool {
	_, ok := blockTags[tag]
	return ok
}

func TestNormalizeHTML(t *testing.T) {
	tests := []struct {
		b    string
		want string
	}{
		{"<p>a  \t b</p>", "<p>a b</p>"},
		{"<p>a  \t\nb</p>", "<p>a b</p>"},
		{"<p>a  b</p>", "<p>a b</p>"},
		{" <p>a  b</p>", "<p>a b</p>"},
		{"<p>a  b</p> ", "<p>a b</p>"},
		{"\n\t<p>\n\t\ta  b\t\t</p>\n\t", "<p>a b</p>"},
		{"<i>a  b</i> ", "<i>a b</i> "},
		{"<br />", "<br>"},
		{`<a title="bar" HREF="foo">x</a>`, `<a href="foo" title="bar">x</a>`},
		{"&forall;&amp;&gt;&lt;&quot;", "\u2200&amp;&gt;&lt;&quot;"},
	}
	for _, test := range tests {
		if got := normalizeHTML([]byte(test.b)); string(got) != test.want {
			t.Errorf("normalizeHTML(%q) = %q; want %q", test.b, got, test.want)
		}
	}
}
