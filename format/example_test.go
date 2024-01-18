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

package format_test

import (
	"bytes"
	"os"

	"zombiezen.com/go/commonmark"
	"zombiezen.com/go/commonmark/format"
)

func ExampleFormat() {
	blocks, _ := commonmark.Parse([]byte(`
 Hello, World!


This is a very loose document with:
  - Wildly indented blocks
 - Shortcut reference [links] (that will be maintained but rewritten as collapsed reference links),
   collapsed reference [links][], ordinary [reference links][ LINKS ],
   along with [inline links]( https://www.example.com/ "with titles" )

[ links ]: https://www.example.com/
`))
	out := new(bytes.Buffer)
	if err := format.Format(out, blocks); err != nil {
		// Writing in-memory shouldn't fail.
		panic(err)
	}
	os.Stdout.Write(out.Bytes())
	// Output:
	// Hello, World!
	//
	// This is a very loose document with:
	//
	// - Wildly indented blocks
	// - Shortcut reference [links][] (that will be maintained but rewritten as collapsed reference links),
	//   collapsed reference [links][], ordinary [reference links][links],
	//   along with [inline links](https://www.example.com/ "with titles")
	//
	// [links]: https://www.example.com/
}
