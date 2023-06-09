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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseThematicBreak(t *testing.T) {
	tests := []struct {
		line string
		want int
	}{
		{"", -1},
		{"---\n", 3},
		{"***\n", 3},
		{"___\n", 3},
		{"+++\n", -1},
		{"===\n", -1},
		{"--\n", -1},
		{"**\n", -1},
		{"__\n", -1},
		{"_____________________________________\n", 37},
		{"- - -\n", 5},
		{"**  * ** * ** * **\n", 18},
		{"-     -      -      -\n", 21},
		{"- - - -    \n", 7},
		{"_ _ _ _ a\n", -1},
		{"a------\n", -1},
		{"---a---\n", -1},
		{"*-*\n", -1},
	}
	for _, test := range tests {
		if got := parseThematicBreak([]byte(test.line)); got != test.want {
			t.Errorf("parseThematicBreak(%q) = %d; want %d", test.line, got, test.want)
		}
	}
}

func TestParseATXHeading(t *testing.T) {
	tests := []struct {
		line string
		want atxHeading
	}{
		{"# foo\n", atxHeading{level: 1, content: Span{Start: 2, End: 5}}},
		{"## foo\n", atxHeading{level: 2, content: Span{Start: 3, End: 6}}},
		{"### foo\n", atxHeading{level: 3, content: Span{Start: 4, End: 7}}},
		{"#### foo\n", atxHeading{level: 4, content: Span{Start: 5, End: 8}}},
		{"##### foo\n", atxHeading{level: 5, content: Span{Start: 6, End: 9}}},
		{"###### foo\n", atxHeading{level: 6, content: Span{Start: 7, End: 10}}},
		{"####### foo\n", atxHeading{}},
		{"#5 bolt\n", atxHeading{}},
		{"#hashtag\n", atxHeading{}},
		{"\\## foo\n", atxHeading{}},
		{"# foo *bar* \\*baz\\*\n", atxHeading{level: 1, content: Span{Start: 2, End: 19}}},
		{
			"#                  foo                     \n",
			atxHeading{level: 1, content: Span{Start: 19, End: 22}},
		},
		{"## foo ##\n", atxHeading{level: 2, content: Span{Start: 3, End: 6}}},
		{"# foo ##################################\n", atxHeading{level: 1, content: Span{Start: 2, End: 5}}},
		{"##### foo ##\n", atxHeading{level: 5, content: Span{Start: 6, End: 9}}},
		{"### foo ###     \n", atxHeading{level: 3, content: Span{Start: 4, End: 7}}},
		{"### foo ### b\n", atxHeading{level: 3, content: Span{Start: 4, End: 13}}},
		{"# foo#\n", atxHeading{level: 1, content: Span{Start: 2, End: 6}}},
		{"### foo \\###\n", atxHeading{level: 3, content: Span{Start: 4, End: 12}}},
		{"## foo #\\##\n", atxHeading{level: 2, content: Span{Start: 3, End: 11}}},
		{"# foo \\#\n", atxHeading{level: 1, content: Span{Start: 2, End: 8}}},
		{"## \n", atxHeading{level: 2, content: Span{Start: 3, End: 3}}},
		{"#\n", atxHeading{level: 1, content: Span{Start: 1, End: 1}}},
		{"### ###\n", atxHeading{level: 3, content: Span{Start: 4, End: 4}}},

		{"# foo \\  #\n", atxHeading{level: 1, content: Span{Start: 2, End: 8}}},
	}
	for _, test := range tests {
		got := parseATXHeading([]byte(test.line))
		if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(atxHeading{})); diff != "" {
			t.Errorf("parseATXHeading(%q) (-want +got):\n%s", test.line, diff)
		}
	}
}
