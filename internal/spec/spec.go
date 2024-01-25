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

// Package spec provides access to the examples from the CommonMark specification.
package spec

import (
	_ "embed"
	"encoding/json"
)

// Example is a single example from the specification.
type Example struct {
	Markdown string
	HTML     string
	Example  int
	Section  string
}

//go:embed spec-0.30.json
var specData []byte

// Load returns the examples from the CommonMark specification.
func Load() ([]Example, error) {
	var testsuite []Example
	if err := json.Unmarshal(specData, &testsuite); err != nil {
		return nil, err
	}
	return testsuite, nil
}

//go:embed spec-0.29.0.gfm.11.json
var gfmSpecData []byte

// LoadGFM returns the examples from the GitHub-Flavored Markdown specification.
func LoadGFM() ([]Example, error) {
	var testsuite []Example
	if err := json.Unmarshal(gfmSpecData, &testsuite); err != nil {
		return nil, err
	}
	return testsuite, nil
}
