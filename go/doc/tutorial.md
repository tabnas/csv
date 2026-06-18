# Tutorial — your first CSV parse (Go)

This walks you from nothing to a working parse, then through the four
shapes of program you are most likely to need: a basic parse, type
coercion, quoted fields, and streaming. Follow it in order; each step
builds on the last.

For a recipe-style index of individual tasks, see the
[how-to guide](guide.md). For exhaustive signatures and every option,
see the [reference](reference.md). For how it works under the hood —
including how the Go API differs from the TypeScript original — see
[concepts](concepts.md).

## 1. Install

```bash
go get github.com/tabnas/csv/go
```

The module path is `github.com/tabnas/csv/go` and is normally imported
with the alias `csv`. It depends on `github.com/tabnas/jsonic/go` for
the underlying parser.

```go
import (
    csv "github.com/tabnas/csv/go"
    jsonic "github.com/tabnas/jsonic/go"
)
```

A configured parser is one Jsonic instance with the CSV plugin
registered:

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults)

result, err := j.Parse("a,b\n1,2")
```

`UseDefaults` merges any extra `map[string]any` arguments on top of
`csv.Defaults`, so you only specify what differs from the default.

## 2. Parse a two-line CSV

```go
package main

import (
    "fmt"

    csv "github.com/tabnas/csv/go"
    jsonic "github.com/tabnas/jsonic/go"
)

func main() {
    j := jsonic.Make()
    j.UseDefaults(csv.Csv, csv.Defaults)

    result, err := j.Parse("name,age\nAlice,30\nBob,25")
    if err != nil {
        panic(err)
    }

    for _, r := range result.([]any) {
        row := r.(map[string]any)
        fmt.Printf("%s is %s\n", row["name"], row["age"])
    }
    // Alice is 30
    // Bob is 25
}
```

The first row was treated as a header (the default), and each
subsequent row became a `map[string]any` keyed by those names. `30` and
`25` are *strings* — strict mode is on by default, and strict mode keeps
every field as the raw text it appeared as.

## 3. Turn the strings into numbers

Strict mode treats every cell as raw text. Enable `number` (and
optionally `value` for the literals `true`, `false`, `null`):

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "number": true,
    "value":  true,
})

result, _ := j.Parse("name,age,active\nAlice,30,true\nBob,25,false")
// [
//   {name: Alice, age: 30, active: true},
//   {name: Bob,   age: 25, active: false},
// ]
//
// where 30, 25 are float64 and true, false are bool.
```

These options are independent — turn on whichever ones the data calls
for.

## 4. Quoted fields with commas and newlines

CSV's quote rules are the bit you have to deal with eventually. Wrap a
field in `"` to include commas, newlines, or quotes; double a quote
(`""`) to escape it.

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults)

src := `name,bio
Alice,"Likes ""cats"" and dogs"
Bob,"line 1
line 2"`

result, _ := j.Parse(src)
// [
//   {name: Alice, bio: Likes "cats" and dogs},
//   {name: Bob,   bio: "line 1\nline 2"},
// ]
```

The plugin ships its own quoted-string lexer to follow RFC 4180 (see
[Concepts: Quoted fields](concepts.md#quoted-fields-rfc-4180)).

## 5. Stream records to a callback

If the input is too big to hold in memory, register a `stream`
callback. The plugin emits one event per record and the top-level
result is `[]any{}`.

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "stream": func(what string, payload any) {
        if what == "record" {
            fmt.Println("row:", payload)
        }
    },
})

j.Parse("a,b\n1,2\n3,4")
// row: map[a:1 b:2]
// row: map[a:3 b:4]
```

You now have the four shapes — basic parse, type coercion, quoted data,
streaming. Everything else is variations on these.

## Where to go next

- [How-to guide](guide.md) — focused recipes for individual tasks.
- [Reference](reference.md) — the public API, every option, and the grammar.
- [Concepts](concepts.md) — how the plugin works, and how Go differs from TS.
