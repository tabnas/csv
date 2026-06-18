# @tabnas/csv

A [Jsonic](https://github.com/tabnas/jsonic) / [Tabnas](https://github.com/tabnas/parser)
syntax plugin that parses CSV text into objects or arrays, with support
for headers, quoted fields, custom delimiters, streaming, and
strict/non-strict modes.


[![npm version](https://img.shields.io/npm/v/@tabnas/csv.svg)](https://npmjs.com/package/@tabnas/csv)
[![build](https://github.com/tabnas/csv/actions/workflows/build.yml/badge.svg)](https://github.com/tabnas/csv/actions/workflows/build.yml)
[![Coverage Status](https://coveralls.io/repos/github/tabnas/csv/badge.svg?branch=main)](https://coveralls.io/github/tabnas/csv?branch=main)
[![Known Vulnerabilities](https://snyk.io/test/github/tabnas/csv/badge.svg)](https://snyk.io/test/github/tabnas/csv)
[![DeepScan grade](https://deepscan.io/api/teams/5016/projects/22466/branches/663906/badge/grade.svg)](https://deepscan.io/dashboard#view=project&tid=5016&pid=22466&bid=663906)
[![Maintainability](https://api.codeclimate.com/v1/badges/10e9bede600896c77ce8/maintainability)](https://codeclimate.com/github/tabnas/csv/maintainability)

| ![Voxgig](https://www.voxgig.com/res/img/vgt01r.png) | This open source module is sponsored and supported by [Voxgig](https://www.voxgig.com). |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------- |


## Install

```bash
npm install @tabnas/csv @tabnas/parser @tabnas/jsonic
```

`@tabnas/parser` and `@tabnas/jsonic` are peer dependencies: you create
a Tabnas instance, load the jsonic grammar, then register the plugin.


## One tiny example

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv)

parse.parse('name,age\nAlice,30\nBob,25') // => [{ name: 'Alice', age: '30' }, { name: 'Bob', age: '25' }]
```


## Documentation

Full documentation, in the four [Diátaxis](https://diataxis.fr)
quadrants:

- [Tutorial](doc/tutorial.md) — a guided first parse.
- [How-to guide](doc/guide.md) — task recipes (delimiters, headers, streaming, …).
- [Reference](doc/reference.md) — the public API, every option, and the grammar.
- [Concepts](doc/concepts.md) — how the plugin works on the engine, and why.

For the Go port, see [`../go/doc/`](../go/doc/).


## Grammar diagram

The grammar is defined in the repo-root
[`csv-grammar.jsonic`](../csv-grammar.jsonic) and embedded into
[`src/csv.ts`](src/csv.ts) (and the Go port) by
[`embed-grammar.js`](embed-grammar.js) during `npm run build`.

The live grammar as a railroad/syntax diagram, generated with
[`@tabnas/railroad`](https://github.com/tabnas/railroad):

![csv grammar railroad diagram](doc/grammar.svg)

A vertical ASCII version is in [`doc/grammar.txt`](doc/grammar.txt).

## License

Copyright (c) 2021-2025 Richard Rodger and other contributors,
[MIT License](LICENSE).
