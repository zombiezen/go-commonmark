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

package normhtml

import "testing"

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
		if got := NormalizeHTML([]byte(test.b)); string(got) != test.want {
			t.Errorf("NormalizeHTML(%q) = %q; want %q", test.b, got, test.want)
		}
	}
}
