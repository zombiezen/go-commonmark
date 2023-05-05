# `zombiezen.com/go/commonmark` Release Notes

The format is based on [Keep a Changelog][],
and this project adheres to [Semantic Versioning][].

[Keep a Changelog]: https://keepachangelog.com/en/1.0.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
[Unreleased]: https://github.com/zombiezen/go-commonmark/compare/v0.2.0...HEAD

## [Unreleased][]

### Added

- HTML rendering can be customized with the new `HTMLRenderer` type.
  ([#2](https://github.com/zombiezen/go-commonmark/issues/2))


### Fixed

- HTML rendering now performs significantly less allocations.
- Soft line breaks are now being emitted correctly.
  They previously were being emitted as part of the text
  except in specific conditions of code blocks.

## [0.2.0][] - 2023-04-30

Version 0.2 includes more documentation, adds some small features,
and fixes some serious bugs in the parser.

[0.2.0]: https://github.com/zombiezen/go-commonmark/releases/tag/v0.2.0

### Added

- Added `RootBlock.EndOffset` to assist in mapping blocks with NUL bytes
  back to original source.
- Finished documentation of all exported symbols.
- Added `BlockKind.String` and `InlineKind.String` methods.
- Added `Node.Span` method.

### Changed

- Indentation is now preserved verbatim in the parse tree
  (i.e. a `TextKind` inline covers the spaces rather than an `IndentKind`)
  in more cases.

### Fixed

- Link/image inline spans are now correct.
- Fixed an infinite loop that occurred when a document ended in backticks.

## [0.1.0][] - 2023-04-23

Initial public release.
Complies with the [CommonMark 0.30.0 specification](https://spec.commonmark.org/0.30/).

[0.1.0]: https://github.com/zombiezen/go-commonmark/releases/tag/v0.1.0
