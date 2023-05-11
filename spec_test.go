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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"zombiezen.com/go/commonmark/internal/normhtml"
)

func TestSpec(t *testing.T) {
	for _, test := range loadTestSuite(t) {
		t.Run(fmt.Sprintf("Example%d", test.Example), func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.Markdown))
			buf := new(bytes.Buffer)
			if err := RenderHTML(buf, blocks, refMap); err != nil {
				t.Error("RenderHTML:", err)
			}
			got := string(normhtml.NormalizeHTML(buf.Bytes()))
			want := string(normhtml.NormalizeHTML([]byte(test.HTML)))
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", test.Markdown, diff)
			}
		})
	}
}

func TestGFMSpec(t *testing.T) {
	t.Skip("GitHub Flavored Markdown not supported")

	data, err := os.ReadFile(filepath.Join("testdata", "spec-0.29.0.gfm.11.json"))
	if err != nil {
		t.Fatal(err)
	}
	var testsuite []specExample
	if err := json.Unmarshal(data, &testsuite); err != nil {
		t.Fatal(err)
	}

	for _, test := range testsuite {
		t.Run(fmt.Sprintf("Example%d", test.Example), func(t *testing.T) {
			blocks, refMap := Parse([]byte(test.Markdown))
			buf := new(bytes.Buffer)
			if err := RenderHTML(buf, blocks, refMap); err != nil {
				t.Error("RenderHTML:", err)
			}
			got := string(normhtml.NormalizeHTML(buf.Bytes()))
			want := string(normhtml.NormalizeHTML([]byte(test.HTML)))
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("Input:\n%s\nOutput (-want +got):\n%s", test.Markdown, diff)
			}
		})
	}
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
		got := string(normhtml.NormalizeHTML(buf.Bytes()))

		c := exec.Command(commonmarkArgs[0], commonmarkArgs[1:]...)
		c.Stdin = strings.NewReader(markdown)
		c.Stderr = os.Stderr
		rawWant, err := c.Output()
		if err != nil {
			t.Fatal(err)
		}
		want := string(normhtml.NormalizeHTML(rawWant))

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
