// The translation part: what the manifest says and what the crate embeds
// are the same file.
//
// A packaged crate holds nothing outside `rs/`, so the crate embeds its
// own copy of `tabnas.plugin.json`, `rs/translate/manifest.json`, as
// `manifest_text()`. The copy is the only text a host sees, so it must be
// the file: this holds the embedded manifest to the repository's. Change
// the manifest at the root and copy it into `rs/translate/`; this fails
// until both are the same. CSV's render is the `csv` render alchemy
// carries, not a file of this repository's, so there is no render to hold
// and no `render_text()`.

mod common;

use std::fs;

use serde_json::Value;

fn translate() -> Value {
    let manifest: Value =
        serde_json::from_str(tabnas_csv::manifest_text()).expect("the manifest is JSON");
    manifest
        .get("translate")
        .cloned()
        .expect("the manifest carries a translate object")
}

#[test]
fn the_manifest_the_crate_embeds_is_the_repositorys() {
    let on_disk = fs::read_to_string(common::repo_root().join("tabnas.plugin.json"))
        .expect("the repository has its manifest");
    assert_eq!(
        on_disk,
        tabnas_csv::manifest_text(),
        "rs/translate/manifest.json is not tabnas.plugin.json: copy the manifest into rs/translate"
    );
}

/// CSV is read as a tree (one object per record, keyed by the header) and
/// written from records, through the `csv` render alchemy carries. Its
/// events carry its records already, so there is no lift, and no render
/// file of its own.
#[test]
fn csv_reads_a_tree_and_writes_records_through_the_csv_render() {
    let translate = translate();
    assert_eq!(translate["reads"], "tree");
    assert_eq!(translate["writes"], "records");
    assert_eq!(translate["render"], "csv");
    assert_eq!(translate.get("lift"), None);
}

/// The host prints the loss lines verbatim, so each is a sentence.
#[test]
fn the_loss_is_a_list_of_sentences() {
    let translate = translate();
    let loss = translate["loss"]
        .as_array()
        .expect("translate.loss is a list");
    assert!(!loss.is_empty());
    for line in loss {
        let line = line.as_str().expect("each loss line is a string");
        assert!(
            line.starts_with(char::is_uppercase) && line.ends_with('.'),
            "{line:?} is not a sentence"
        );
    }
}
