# @tabnas/csv

A [Jsonic](https://github.com/tabnas/jsonic) / [Tabnas](https://github.com/tabnas/parser)
grammar plugin that parses CSV text into arrays of objects (or arrays
of arrays), with headers, RFC 4180 quoting, custom field/record
separators, streaming, and a strict / non-strict mode. Available for
both TypeScript and Go.

## Install

```bash
# TypeScript
npm install @tabnas/csv @tabnas/parser @tabnas/jsonic
```

```bash
# Go
go get github.com/tabnas/csv/go
```

## One tiny example

**TypeScript**

```typescript
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv)

parse.parse('name,age\nAlice,30\nBob,25')
// [{ name: 'Alice', age: '30' }, { name: 'Bob', age: '25' }]
```

**Go**

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

## Documentation

Full documentation follows the [Diátaxis](https://diataxis.fr) four
quadrants — one file each for learning, doing, looking up, and
understanding.

**TypeScript** — [`ts/doc/`](ts/doc/)

- [Tutorial](ts/doc/tutorial.md) — a guided first parse.
- [How-to guide](ts/doc/guide.md) — task recipes.
- [Reference](ts/doc/reference.md) — API, options, and grammar.
- [Concepts](ts/doc/concepts.md) — how it works, and why.

**Go** — [`go/doc/`](go/doc/)

- [Tutorial](go/doc/tutorial.md) — a guided first parse.
- [How-to guide](go/doc/guide.md) — task recipes.
- [Reference](go/doc/reference.md) — API, options, and grammar.
- [Concepts](go/doc/concepts.md) — how it works, plus differences from TS.

## Repository layout

| Path | Description |
|---|---|
| [`ts/`](ts/) | TypeScript / JavaScript implementation (`@tabnas/csv`). |
| [`go/`](go/) | Go port (`github.com/tabnas/csv/go`). |
| [`csv-grammar.jsonic`](csv-grammar.jsonic) | The grammar, embedded into both runtimes. |
| [`test/fixtures/`](test/fixtures/) | Shared conformance fixtures, exercised by both runtimes. |

## Grammar

The grammar is defined once in the top-level
[`csv-grammar.jsonic`](csv-grammar.jsonic) and embedded into both the
TypeScript ([`ts/src/csv.ts`](ts/src/csv.ts)) and Go
([`go/csv.go`](go/csv.go)) implementations by
[`ts/embed-grammar.js`](ts/embed-grammar.js) (run as part of
`npm run build`).

## Grammar diagram

The live grammar as a railroad/syntax diagram, generated with
[`@tabnas/railroad`](https://github.com/tabnas/railroad):

![csv grammar railroad diagram](ts/doc/grammar.svg)

ASCII version: [`ts/doc/grammar.txt`](ts/doc/grammar.txt).

## License

MIT. Copyright (c) Richard Rodger and other contributors.
