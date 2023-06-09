// Code generated by "stringer -type=BlockKind,InlineKind -output=kind_string.go"; DO NOT EDIT.

package commonmark

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[ParagraphKind-1]
	_ = x[ThematicBreakKind-2]
	_ = x[ATXHeadingKind-3]
	_ = x[SetextHeadingKind-4]
	_ = x[IndentedCodeBlockKind-5]
	_ = x[FencedCodeBlockKind-6]
	_ = x[HTMLBlockKind-7]
	_ = x[LinkReferenceDefinitionKind-8]
	_ = x[BlockQuoteKind-9]
	_ = x[ListItemKind-10]
	_ = x[ListKind-11]
	_ = x[ListMarkerKind-12]
	_ = x[documentKind-13]
}

const _BlockKind_name = "ParagraphKindThematicBreakKindATXHeadingKindSetextHeadingKindIndentedCodeBlockKindFencedCodeBlockKindHTMLBlockKindLinkReferenceDefinitionKindBlockQuoteKindListItemKindListKindListMarkerKinddocumentKind"

var _BlockKind_index = [...]uint8{0, 13, 30, 44, 61, 82, 101, 114, 141, 155, 167, 175, 189, 201}

func (i BlockKind) String() string {
	i -= 1
	if i >= BlockKind(len(_BlockKind_index)-1) {
		return "BlockKind(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _BlockKind_name[_BlockKind_index[i]:_BlockKind_index[i+1]]
}
func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[TextKind-1]
	_ = x[SoftLineBreakKind-2]
	_ = x[HardLineBreakKind-3]
	_ = x[IndentKind-4]
	_ = x[CharacterReferenceKind-5]
	_ = x[InfoStringKind-6]
	_ = x[EmphasisKind-7]
	_ = x[StrongKind-8]
	_ = x[LinkKind-9]
	_ = x[ImageKind-10]
	_ = x[LinkDestinationKind-11]
	_ = x[LinkTitleKind-12]
	_ = x[LinkLabelKind-13]
	_ = x[CodeSpanKind-14]
	_ = x[AutolinkKind-15]
	_ = x[HTMLTagKind-16]
	_ = x[RawHTMLKind-17]
	_ = x[UnparsedKind-18]
}

const _InlineKind_name = "TextKindSoftLineBreakKindHardLineBreakKindIndentKindCharacterReferenceKindInfoStringKindEmphasisKindStrongKindLinkKindImageKindLinkDestinationKindLinkTitleKindLinkLabelKindCodeSpanKindAutolinkKindHTMLTagKindRawHTMLKindUnparsedKind"

var _InlineKind_index = [...]uint8{0, 8, 25, 42, 52, 74, 88, 100, 110, 118, 127, 146, 159, 172, 184, 196, 207, 218, 230}

func (i InlineKind) String() string {
	i -= 1
	if i >= InlineKind(len(_InlineKind_index)-1) {
		return "InlineKind(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _InlineKind_name[_InlineKind_index[i]:_InlineKind_index[i+1]]
}
