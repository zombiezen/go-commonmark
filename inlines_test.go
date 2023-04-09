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

import "testing"

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
