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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go4.org/bytereplacer"
	"golang.org/x/net/html"
	. "zombiezen.com/go/commonmark"
)

var supportedSections = map[string]struct{}{
	"Tabs":            {},
	"Thematic breaks": {},
	"Paragraphs":      {},
	"ATX headings":    {},
	"Block quotes":    {},
	"Lists":           {},
}

var skippedExamples = map[int]string{
	56:  "emphasis not implemented",
	59:  "setext headings not implemented",
	65:  "uses inline backslash escapes",
	66:  "emphasis not implemented",
	76:  "uses inline backslash escapes",
	226: "needs hard line breaks",
	237: "fenced code blocks not implemented",
	308: "HTML comments not implemented",
	309: "HTML comments not implemented",
	317: "link reference definitions not implemented",
	318: "fenced code blocks not implemented",
	321: "fenced code blocks not implemented",
	324: "fenced code blocks not implemented",
	325: "tightness buggy",
	326: "tightness buggy",
}

func TestSpec(t *testing.T) {
	testsuiteData, err := os.ReadFile(filepath.Join("testdata", "spec-0.30.json"))
	if err != nil {
		t.Fatal(err)
	}
	var testsuite []struct {
		Markdown string
		HTML     string
		Example  int
		Section  string
	}
	if err := json.Unmarshal(testsuiteData, &testsuite); err != nil {
		t.Fatal(err)
	}

	for _, test := range testsuite {
		t.Run(fmt.Sprintf("Example%d", test.Example), func(t *testing.T) {
			if _, ok := supportedSections[test.Section]; !ok {
				t.Skipf("Section %q not implemented yet", test.Section)
			}
			if skipReason := skippedExamples[test.Example]; skipReason != "" {
				t.Skip("Skipped:", skipReason)
			}
			blocks := Parse([]byte(test.Markdown))
			buf := new(bytes.Buffer)
			if err := RenderHTML(buf, blocks); err != nil {
				t.Error("RenderHTML:", err)
			}
			got := string(normalizeHTML(buf.Bytes()))
			want := string(normalizeHTML([]byte(test.HTML)))
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("-want +got:\n%s", diff)
			}
		})
	}
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