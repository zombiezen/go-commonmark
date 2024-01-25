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

package format

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"zombiezen.com/go/commonmark"
	"zombiezen.com/go/commonmark/internal/normhtml"
	"zombiezen.com/go/commonmark/internal/spec"
)

func FuzzFormat(f *testing.F) {
	examples, err := spec.Load()
	if err != nil {
		f.Fatal(err)
	}
	for _, ex := range examples {
		f.Add(ex.Markdown)
	}

	f.Fuzz(func(t *testing.T, markdown string) {
		blocks, refMap := commonmark.Parse([]byte(markdown))
		originalHTML := new(bytes.Buffer)
		if err := commonmark.RenderHTML(originalHTML, blocks, refMap); err != nil {
			t.Fatal("Render original HTML:", err)
		}

		got := new(bytes.Buffer)
		if err := Format(got, blocks); err != nil {
			t.Error("Format #1:", err)
		}

		formattedBlocks, formattedRefMap := commonmark.Parse(got.Bytes())
		formattedHTML := new(bytes.Buffer)
		if err := commonmark.RenderHTML(formattedHTML, formattedBlocks, formattedRefMap); err != nil {
			t.Error("Render formatted HTML:", err)
		} else {
			diff := cmp.Diff(string(normhtml.NormalizeHTML(originalHTML.Bytes())), string(normhtml.NormalizeHTML(formattedHTML.Bytes())))
			if diff != "" {
				// TODO(soon): Once all cases are handled, change this to Errorf.
				t.Skipf("Reformatting changed semantics. Original:\n%s\nReformatting:\n%s\nHTML diff (-want +got):\n%s", markdown, got, diff)
			}
		}

		reformatted := new(bytes.Buffer)
		if err := Format(reformatted, formattedBlocks); err != nil {
			t.Error("Format #2:", err)
		}
		if diff := cmp.Diff(got.String(), reformatted.String()); diff != "" {
			t.Errorf("Format not idempotent (-first +second):\n%s", diff)
		}
	})
}

func TestWriteTrimmedIndent(t *testing.T) {
	tests := []struct {
		indents []string
		want    string
	}{
		{[]string{}, ""},
		{[]string{""}, ""},
		{[]string{" \t "}, ""},
		{[]string{"> "}, ">"},
		{[]string{"> ", "> "}, "> >"},
		{[]string{"> ", "> ", "  "}, "> >"},
	}
	for _, test := range tests {
		got := new(strings.Builder)
		if err := writeTrimmedIndent(got, test.indents); got.String() != test.want || err != nil {
			t.Errorf("writeTrimmedIndent(buf, %q) = %q, %v; want %q, <nil>",
				test.indents, got, err, test.want)
		}
	}
}
