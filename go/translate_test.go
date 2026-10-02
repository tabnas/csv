package tabnascsv

import (
	"os"
	"testing"
)

func TestTranslationParts(t *testing.T) {
	parts := Translate()
	if parts == nil {
		t.Fatal("Translate returned nil")
	}
	manifest, err := os.ReadFile("../tabnas.plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if parts.Manifest != string(manifest) {
		t.Fatal("embedded manifest differs from tabnas.plugin.json")
	}
	if parts.Lift != nil {
		t.Fatal("CSV has no lift")
	}
	if parts.Render == nil || parts.Render.Entry != "csv" {
		t.Fatalf("render entry is %#v", parts.Render)
	}
	if parts.Render.Source != "" {
		t.Fatal("alchemy's builtin CSV render has no package source")
	}
}
