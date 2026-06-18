# Tutorial — your first CSV parse (TypeScript)

This walks you from nothing to a working parse, then through the four
shapes of program you are most likely to need: a basic parse, type
coercion, quoted fields, and streaming. Follow it in order; each step
builds on the last.

For a recipe-style index of individual tasks, see the
[how-to guide](guide.md). For exhaustive signatures and every option,
see the [reference](reference.md). For how it works under the hood,
see [concepts](concepts.md).

## 1. Install

```bash
npm install @tabnas/csv @tabnas/parser @tabnas/jsonic
```

`@tabnas/parser` is the parsing engine and `@tabnas/jsonic` (>= 2)
provides the base JSON grammar; both are peer dependencies. The plugin
re-uses the parser's lexer and the jsonic grammar, so you always create
a Tabnas instance, load the jsonic grammar, then register the plugin on
it.

```typescript
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv)
```

`parse` is now a Tabnas instance whose `.parse()` method accepts a CSV
string and returns the parsed result. Reuse it for as many inputs as
you need — each call is independent.

## 2. Parse a two-line CSV

Make a Tabnas instance, register the plugin, and call it with a string:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv)

parse.parse('name,age\nAlice,30\nBob,25') // => [{ name: 'Alice', age: '30' }, { name: 'Bob', age: '25' }]
```

The first row was treated as a header (this is the default), and each
subsequent row became an object keyed by those names. Note that `30`
and `25` are *strings* — strict mode is on, and strict mode keeps every
field as the raw text it appeared as.

## 3. Turn the numbers into numbers

Strict mode treats CSV as data, not jsonic source. To opt in to type
coercion, enable `number` (and optionally `value` for the literals
`true`, `false`, `null`):

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { number: true, value: true })

parse.parse('name,age,active\nAlice,30,true\nBob,25,false') // => [{ name: 'Alice', age: 30, active: true }, { name: 'Bob', age: 25, active: false }]
```

These options are independent — turn on whichever ones the data calls
for.

## 4. Quoted fields with commas and newlines

CSV's quote rules are the bit everybody has to deal with eventually.
Wrap a field in `"` to include commas, newlines, or quotes in the
value; double a quote (`""`) to escape it.

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv)

parse.parse('name,bio\nAlice,"Likes ""cats"" and dogs"\nBob,"line 1\nline 2"') // => [{ name: 'Alice', bio: 'Likes "cats" and dogs' }, { name: 'Bob', bio: 'line 1\nline 2' }]
```

The plugin ships its own quoted-string lexer to follow RFC 4180 quoting
precisely (see [Concepts: Quoted fields](concepts.md#quoted-fields-rfc-4180)).

## 5. Stream a large file

If the input is too big to hold in memory, supply a `stream` callback.
The plugin emits one event per record and the top-level result is `[]`:

```typescript
const parse = new Tabnas().use(jsonic).use(Csv, {
  number: true,
  stream: (event, payload) => {
    if (event === 'record') {
      // payload is one parsed record
      console.log(payload)
    } else if (event === 'error') {
      console.error(payload)
    }
  },
})

// Parse the whole string in one shot — records flow through the
// callback as they are produced.
parse.parse('a,b\n1,2\n3,4')
```

You now have the four shapes — basic parse, type coercion, quoted data,
streaming. Everything else is variations on these.

## Where to go next

- [How-to guide](guide.md) — focused recipes for individual tasks.
- [Reference](reference.md) — the public API, every option, and the grammar.
- [Concepts](concepts.md) — how the plugin works on the engine, and why.
