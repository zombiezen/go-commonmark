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

// Package normhtml provides a function for normalizing HTML
// which ignores insignificant output differences,
// based on the [CommonMark spec test normalization].
//
// [CommonMark spec test normalization]: https://github.com/commonmark/commonmark-spec/blob/0.30.0/test/normalize.py
package normhtml

import (
	"bytes"
	"regexp"
	"sort"
	"unicode"

	"go4.org/bytereplacer"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var whitespaceRE = regexp.MustCompile(`\s+`)

var htmlEscaper = bytereplacer.New(
	"&", "&amp;",
	`'`, "&apos;",
	`<`, "&lt;",
	`>`, "&gt;",
	`"`, "&quot;",
)

// NormalizeHTML strips insignificant output differences from HTML.
func NormalizeHTML(b []byte) []byte {
	type htmlAttribute struct {
		key   string
		value string
	}

	tok := html.NewTokenizerFragment(bytes.NewReader(b), "div")
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
	atom.Article.String():    {},
	atom.Header.String():     {},
	atom.Aside.String():      {},
	atom.Hgroup.String():     {},
	atom.Blockquote.String(): {},
	atom.Hr.String():         {},
	atom.Iframe.String():     {},
	atom.Body.String():       {},
	atom.Li.String():         {},
	atom.Map.String():        {},
	atom.Button.String():     {},
	atom.Object.String():     {},
	atom.Canvas.String():     {},
	atom.Ol.String():         {},
	atom.Caption.String():    {},
	atom.Output.String():     {},
	atom.Col.String():        {},
	atom.P.String():          {},
	atom.Colgroup.String():   {},
	atom.Pre.String():        {},
	atom.Dd.String():         {},
	atom.Progress.String():   {},
	atom.Div.String():        {},
	atom.Section.String():    {},
	atom.Dl.String():         {},
	atom.Table.String():      {},
	atom.Td.String():         {},
	atom.Dt.String():         {},
	atom.Tbody.String():      {},
	atom.Embed.String():      {},
	atom.Textarea.String():   {},
	atom.Fieldset.String():   {},
	atom.Tfoot.String():      {},
	atom.Figcaption.String(): {},
	atom.Th.String():         {},
	atom.Figure.String():     {},
	atom.Thead.String():      {},
	atom.Footer.String():     {},
	atom.Tr.String():         {},
	atom.Form.String():       {},
	atom.Ul.String():         {},
	atom.H1.String():         {},
	atom.H2.String():         {},
	atom.H3.String():         {},
	atom.H4.String():         {},
	atom.H5.String():         {},
	atom.H6.String():         {},
	atom.Video.String():      {},
	atom.Script.String():     {},
	atom.Style.String():      {},
}

func isBlockTag(tag string) bool {
	_, ok := blockTags[tag]
	return ok
}
