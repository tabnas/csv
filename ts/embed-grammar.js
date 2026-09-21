#!/usr/bin/env node

// Embed csv-grammar.jsonic into the TypeScript, Go AND Rust source files.
// Run via: npm run embed  (or:  node embed-grammar.js)
//
// The grammar text goes in verbatim: each runtime parses it with its own
// jsonic instance at plugin load. Never hand-edit between the BEGIN/END
// markers: edit csv-grammar.jsonic and re-run this script.

const fs = require('fs')
const path = require('path')

const GRAMMAR_FILE = path.join(__dirname, '..', 'csv-grammar.jsonic')
const TS_FILE = path.join(__dirname, 'src', 'csv.ts')
const GO_FILE = path.join(__dirname, '..', 'go', 'csv.go')
const RS_FILE = path.join(__dirname, '..', 'rs', 'src', 'lib.rs')

const BEGIN = '// --- BEGIN EMBEDDED csv-grammar.jsonic ---'
const END = '// --- END EMBEDDED csv-grammar.jsonic ---'

const grammar = fs.readFileSync(GRAMMAR_FILE, 'utf8')

// Replace a source file's contents WITHOUT ever leaving it truncated.
//
// `fs.writeFileSync` opens with O_TRUNC and then writes, so between those
// two steps the file on disk is empty or half-written. Anything reading it
// in that window sees a file with no BEGIN/END markers in it. That window
// is reachable: `make -j build` can run this script while `npm run build`
// runs it too, and `cargo build` / `tsc` / `go build` read the same three
// files. Measured on a copy of the tree, a reader looping over
// `go/csv.go` while this script rewrote it 120 times saw no BEGIN marker
// in 420 of 61,017 reads -- 0.7% -- which is the state that makes a
// second embedder print "Go markers not found" and exit 1.
//
// Writing a sibling temp file and renaming it over the target closes the
// window: rename(2) is atomic within a directory, so a reader sees either
// the whole old file or the whole new one, never a torn one. The embed is
// idempotent, so the usual second run finds identical bytes and does not
// write at all. Re-measured after this change, with the grammar toggled
// between two texts so that every one of the 120 runs really wrote: 0
// bad reads out of 45,118.
function writeAtomic(file, next) {
  if (fs.existsSync(file) && fs.readFileSync(file, 'utf8') === next) {
    return false
  }
  const tmp = file + '.embed-' + process.pid + '.tmp'
  try {
    fs.writeFileSync(tmp, next)
    fs.renameSync(tmp, file)
  } catch (err) {
    try {
      fs.unlinkSync(tmp)
    } catch (ignored) {
      // The temp file was never created, or is already gone.
    }
    throw err
  }
  return true
}

function report(file, written) {
  console.log(
    (written ? 'Embedded grammar into' : 'Grammar already current in'),
    file,
  )
}

// --- TypeScript embedding ---
function embedTS() {
  let src = fs.readFileSync(TS_FILE, 'utf8')
  const startIdx = src.indexOf(BEGIN)
  const endIdx = src.indexOf(END)
  if (startIdx === -1 || endIdx === -1) {
    console.error('TS markers not found in', TS_FILE)
    process.exit(1)
  }

  // Escape backticks and template expressions for a JS template literal.
  const escaped = grammar
    .replace(/\\/g, '\\\\')
    .replace(/`/g, '\\`')
    .replace(/\$\{/g, '\\${')

  const replacement =
    BEGIN +
    '\nconst grammarText = `\n' +
    escaped +
    '`\n' +
    END

  src = src.substring(0, startIdx) + replacement + src.substring(endIdx + END.length)
  report(TS_FILE, writeAtomic(TS_FILE, src))
}

// --- Go embedding ---
function embedGo() {
  let src = fs.readFileSync(GO_FILE, 'utf8')
  const startIdx = src.indexOf(BEGIN)
  const endIdx = src.indexOf(END)
  if (startIdx === -1 || endIdx === -1) {
    console.error('Go markers not found in', GO_FILE)
    process.exit(1)
  }

  if (grammar.includes('`')) {
    console.error('Grammar contains backticks, incompatible with Go raw strings')
    process.exit(1)
  }

  // The blank line before END keeps the result gofmt-clean (gofmt wants
  // a blank line between the const declaration and the trailing comment).
  const replacement =
    BEGIN +
    '\nconst grammarText = `\n' +
    grammar +
    '`\n\n' +
    END

  src = src.substring(0, startIdx) + replacement + src.substring(endIdx + END.length)
  report(GO_FILE, writeAtomic(GO_FILE, src))
}

// --- Rust embedding ---
function embedRust() {
  let src = fs.readFileSync(RS_FILE, 'utf8')
  const startIdx = src.indexOf(BEGIN)
  const endIdx = src.indexOf(END)
  if (startIdx === -1 || endIdx === -1) {
    console.error('Rust markers not found in', RS_FILE)
    process.exit(1)
  }

  // A Rust raw string has no escapes, so the grammar goes in verbatim. The
  // hash count has to clear the longest `"#...` run the text contains, and
  // the grammar is full of `#LN`-style token names, so two hashes is the
  // floor rather than the usual one.
  if (grammar.includes('"##')) {
    console.error('Grammar contains `"##`, incompatible with the r## raw string')
    process.exit(1)
  }

  const replacement =
    BEGIN +
    '\nconst GRAMMAR_TEXT: &str = r##"\n' +
    grammar +
    '"##;\n' +
    END

  src = src.substring(0, startIdx) + replacement + src.substring(endIdx + END.length)
  report(RS_FILE, writeAtomic(RS_FILE, src))
}

embedTS()
embedGo()
if (fs.existsSync(RS_FILE)) {
  embedRust()
} else {
  console.log('No Rust source at', RS_FILE, '- skipping')
}
