# How-to guide (Go)

Short recipes for specific problems. Each is independent; pick the one
that matches the question you have right now. For a guided
introduction, start with the [tutorial](tutorial.md); for exact
signatures and defaults, see the [reference](reference.md).

All examples build on a Jsonic instance:

```go
j := tabnasjsonic.Make()
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, /* overrides */)
result, err := j.Parse(input)
```

## Return arrays instead of maps

Set `object: false` to receive each record as a plain `[]any`. With
`header: true` (the default) the header row is consumed for internal
field tracking and not emitted:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"object": false})

result, _ := j.Parse("a,b,c\n1,2,3\n4,5,6")
// [[1 2 3] [4 5 6]]
```

To get every row including the first, also set `header: false`.

## Use a different field delimiter

Set `field.separation`. Tab, pipe, or any other string — including
multi-character strings such as `"~~"`:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "field": map[string]any{"separation": "\t"},
})

result, _ := j.Parse("name\tage\nAlice\t30")
// [{name: Alice, age: 30}]
```

## Use a different record delimiter

By default a record ends at `\n`, `\r\n`, or `\r`. Override with
`record.separators`:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "record": map[string]any{"separators": "%"},
})

result, _ := j.Parse("a,b%1,2%3,4")
// [{a: 1, b: 2}, {a: 3, b: 4}]
```

## Parse without a header row

If your CSV has no header, set `header: false`. With the default
`object: true` and no field names, the plugin invents keys (`field~0`,
`field~1`, …):

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"header": false})

result, _ := j.Parse("1,2,3")
// [{field~0: 1, field~1: 2, field~2: 3}]
```

## Provide explicit field names

When the file has no header but you still want named fields, set
`header: false` and pass `field.names`:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "header": false,
    "field":  map[string]any{"names": []string{"x", "y", "z"}},
})

result, _ := j.Parse("1,2,3")
// [{x: 1, y: 2, z: 3}]
```

`field.names` is ignored when `object: false` — every row comes out as a
plain `[]any` in that case.

## Trim surrounding whitespace from fields

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"trim": true})

result, _ := j.Parse("a,b\n  hello  ,  world  ")
// [{a: hello, b: world}]
```

Internal whitespace is preserved.

## Skip comment lines

Enable `comment` to strip lines starting with `#`:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"comment": true})

result, _ := j.Parse("a,b\n# this row is ignored\n1,2")
// [{a: 1, b: 2}]
```

A `#` *inside* a field is left alone unless preceded by whitespace.

## Preserve blank lines as empty records

Blank lines are skipped by default. To keep them:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "record": map[string]any{"empty": true},
})

result, _ := j.Parse("a\n1\n\n2")
// [{a: 1}, {a: }, {a: 2}]
```

## Substitute a value for empty fields

Use `field.empty` to set the placeholder for missing cells. Any value
works — string, `nil`, bool, number:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "field": map[string]any{"empty": nil},
})

result, _ := j.Parse("a,b,c\n1,,3")
// [{a: 1, b: <nil>, c: 3}]
```

## Reject rows with the wrong number of fields

`field.exact: true` errors when a record's field count differs from the
header's:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "field": map[string]any{"exact": true},
})

_, err := j.Parse("a,b\n1,2,3")
// err is non-nil
```

The parse halts with a non-nil error. See
[Reference: Errors](reference.md#errors) for a note on the surfaced
error code.

## Allow Jsonic values inside fields

In strict mode, `[1,2]` and `{x:1}` are just text. Switch to non-strict
and jsonic re-engages — you get the parsed value back as a Go value:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"strict": false})

result, _ := j.Parse("a,b,c\ntrue,[1,2],{x:1}")
// [{a: true, b: [1 2], c: {x: 1}}]
```

Non-strict mode also enables `trim`, `comment`, `number`, and `value`
by default. Numbers come back as `float64`.

## Use a different quote character

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "object": false,
    "string": map[string]any{"quote": "'"},
})

result, _ := j.Parse("a,b\n'hi, there','x'")
// [[hi, there x]]
```

## Stream and forward errors

The streaming callback receives `start`, `record`, `end`, and `error`
events. Errors are sent to the callback rather than returned from
`Parse`:

```go
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{
    "object": false,
    "stream": func(what string, payload any) {
        switch what {
        case "record":
            // payload is one row
        case "error":
            // payload is an error value
        case "start", "end":
            // bracket events
        }
    },
})
```

## Reuse the same parser for many inputs

The configured Jsonic instance is reusable — there is no per-call setup
cost beyond the parse itself:

```go
j := tabnasjsonic.Make()
j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, map[string]any{"number": true})

a, _ := j.Parse("x,y\n1,2")
b, _ := j.Parse("p,q\n3,4")
```
