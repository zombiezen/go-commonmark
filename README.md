# `zombiezen.com/go/commonmark`

[![Go Reference](https://pkg.go.dev/badge/zombiezen.com/go/commonmark.svg)][reference docs]

This Go package provides a [CommonMark][] parser,
a specific dialect of [Markdown][].
It allows you to parse, analyze, and render CommonMark/Markdown documents
with an [abstract syntax tree][] API.

This implementation conforms to [Version 0.30 of the CommonMark Specification][].

[abstract syntax tree]: https://en.wikipedia.org/wiki/Abstract_syntax_tree
[CommonMark]: https://commonmark.org/
[Markdown]: https://daringfireball.net/projects/markdown/
[reference docs]: https://pkg.go.dev/zombiezen.com/go/commonmark
[Version 0.30 of the CommonMark Specification]: https://spec.commonmark.org/0.30/

## Goals

A few other Markdown/CommonMark packages exist for Go.
This package prioritizes (in order):

1. Ability to connect the parse tree to CommonMark input losslessly
   in order to enable creation of tools that reformat or manipulate CommonMark documents.
2. Adherence to CommonMark specification.
3. Comprehensibility of implementation.
4. Performance.

## Install

```shell
go get zombiezen.com/go/commonmark
```

## Getting Started

```go
package main

import (
  "fmt"
  "io"
  "os"

  "zombiezen.com/go/commonmark"
)

func main() {
  commonmarkSource, err := io.ReadAll(os.Stdin)
  if err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
  }
  blocks, refMap := commonmark.Parse(commonmarkSource)
  commonmark.RenderHTML(os.Stdout, blocks, refMap)
}
```

## License

[Apache 2.0](LICENSE)
