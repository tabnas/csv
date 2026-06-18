# @tabnas/csv — Go

A [Jsonic](https://jsonic.senecajs.org) syntax plugin that parses CSV
text into Go values, with support for headers, quoted fields, custom
delimiters, streaming, and strict/non-strict modes. This is the Go port
of [`@tabnas/csv`](../ts/); the TypeScript implementation is canonical.

## Install

```bash
go get github.com/tabnas/csv/go
```

The module path is `github.com/tabnas/csv/go`, normally imported with
the alias `csv`. It depends on `github.com/tabnas/jsonic/go` for the
underlying parser.

## One tiny example

```go
import (
    csv "github.com/tabnas/csv/go"
    jsonic "github.com/tabnas/jsonic/go"
)

j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults)

result, _ := j.Parse("name,age\nAlice,30\nBob,25")
// [map[name:Alice age:30] map[name:Bob age:25]]
```

`UseDefaults` merges any extra `map[string]any` arguments on top of
`csv.Defaults`, so you only specify what differs from the default.

## Documentation

Full documentation, in the four [Diátaxis](https://diataxis.fr)
quadrants:

- [Tutorial](doc/tutorial.md) — a guided first parse.
- [How-to guide](doc/guide.md) — task recipes (delimiters, headers, streaming, …).
- [Reference](doc/reference.md) — the public API, every option, and the grammar.
- [Concepts](doc/concepts.md) — how the plugin works, plus differences from TS.

For the canonical TypeScript implementation, see [`../ts/doc/`](../ts/doc/).

## Grammar diagram

The grammar is shared with the TypeScript implementation: it is defined
in the repo-root [`csv-grammar.jsonic`](../csv-grammar.jsonic) and
embedded into [`csv.go`](csv.go) at build time. A railroad/syntax
diagram of the live grammar lives in the TS docs at
[`../ts/doc/grammar.svg`](../ts/doc/grammar.svg) (ASCII version:
[`../ts/doc/grammar.txt`](../ts/doc/grammar.txt)).

## License

Copyright (c) 2021-2025 Richard Rodger and other contributors,
[MIT License](../LICENSE).
