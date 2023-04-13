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

// A type that implements ReferenceMatcher
// can be checked for the presence of link reference definitions.
type ReferenceMatcher interface {
	MatchReference(normalizedLabel string) bool
}

// LinkDefinition is the data of a [link reference definition].
//
// [link reference definition]: https://spec.commonmark.org/0.30/#link-reference-definition
type LinkDefinition struct {
	Destination  string
	Title        string
	TitlePresent bool
}

// ReferenceMap is a mapping of [normalized labels] to link definitions.
//
// [normalized labels]: https://spec.commonmark.org/0.30/#matches
type ReferenceMap map[string]LinkDefinition

// MatchReference reports whether the normalized label appears in the map.
func (m ReferenceMap) MatchReference(normalizedLabel string) bool {
	_, ok := m[normalizedLabel]
	return ok
}

// Extract adds any link reference definitions contained in node to the map.
// In case of conflicts,
// Extract will not replace any existing definitions in the map
// and will use the first definition in source order.
func (m ReferenceMap) Extract(source []byte, node Node) {
	stack := []Node{node}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		block := curr.Block()
		if block == nil {
			continue
		}
		if block.Kind() == LinkReferenceDefinitionKind {
			label := block.inlineChildren[0].LinkReference()
			if _, exists := m[label]; label == "" || exists {
				continue
			}
			def := LinkDefinition{
				Destination:  block.inlineChildren[1].Text(source),
				TitlePresent: len(block.inlineChildren) > 2,
			}
			if def.TitlePresent {
				def.Title = block.inlineChildren[2].Text(source)
			}
			m[label] = def
		} else {
			for i := block.ChildCount() - 1; i >= 0; i-- {
				stack = append(stack, block.Child(i))
			}
		}
	}
}
