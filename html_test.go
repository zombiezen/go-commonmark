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
	"Tabs":                         {},
	"Backslash escapes":            {},
	"Precedence":                   {},
	"Thematic breaks":              {},
	"ATX headings":                 {},
	"Indented code blocks":         {},
	"Fenced code blocks":           {},
	"Paragraphs":                   {},
	"Blank lines":                  {},
	"Block quotes":                 {},
	"List items":                   {},
	"Lists":                        {},
	"Inlines":                      {},
	"Code spans":                   {},
	"Emphasis and strong emphasis": {},
	"Links":                        {},
	"Hard line breaks":             {},
	"Soft line breaks":             {},
	"Textual content":              {},
}

var skippedExamples = map[int]string{
	20:  "autolinks not implemented",
	21:  "raw HTML not implemented",
	23:  "link reference definitions not implemented",
	59:  "setext headings not implemented",
	115: "setext headings not implemented",
	141: "setext headings not implemented",
	226: "needs hard line breaks",
	300: "setext headings not implemented",
	308: "HTML comments not implemented",
	309: "HTML comments not implemented",
	317: "link reference definitions not implemented",
	344: "raw HTML not implemented",
	346: "autolinks not implemented",
	474: "raw HTML not implemented",
	475: "raw HTML not implemented",
	476: "raw HTML not implemented",
	479: "autolinks not implemented",
	480: "autolinks not implemented",
	490: "raw HTML not implemented",
	493: "raw HTML not implemented",
	502: "entity escapes not implemented",
	505: "entity escapes not implemented",
	516: "images not implemented",
	519: "images not implemented",
	523: "raw HTML not implemented",
	525: "autolinks not implemented",
	526: "link reference definitions not implemented",
	527: "link reference definitions not implemented",
	528: "link reference definitions not implemented",
	529: "link reference definitions not implemented",
	530: "images and link reference definitions not implemented",
	531: "link reference definitions not implemented",
	532: "link reference definitions not implemented",
	533: "link reference definitions not implemented",
	534: "link reference definitions not implemented",
	535: "link reference definitions not implemented",
	536: "link reference definitions not implemented",
	537: "link reference definitions not implemented",
	538: "link reference definitions not implemented",
	539: "link reference definitions not implemented",
	540: "link reference definitions not implemented",
	541: "link reference definitions not implemented",
	542: "link reference definitions not implemented",
	543: "link reference definitions not implemented",
	544: "link reference definitions not implemented",
	548: "link reference definitions not implemented",
	549: "link reference definitions not implemented",
	552: "link reference definitions not implemented",
	553: "link reference definitions not implemented",
	554: "link reference definitions not implemented",
	555: "link reference definitions not implemented",
	556: "link reference definitions not implemented",
	557: "link reference definitions not implemented",
	558: "link reference definitions not implemented",
	559: "link reference definitions not implemented",
	560: "link reference definitions not implemented",
	561: "link reference definitions not implemented",
	562: "link reference definitions not implemented",
	563: "link reference definitions not implemented",
	564: "link reference definitions not implemented",
	565: "link reference definitions not implemented",
	566: "link reference definitions not implemented",
	567: "link reference definitions not implemented",
	568: "link reference definitions not implemented",
	569: "link reference definitions not implemented",
	570: "link reference definitions not implemented",
	642: "raw HTML not implemented",
	643: "raw HTML not implemented",
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
				t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", test.Markdown, diff)
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
