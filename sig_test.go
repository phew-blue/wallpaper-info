package main

import "testing"

func TestSigWithChangesWhenPresetChanges(t *testing.T) {
	info := DemoInfo()
	a := DefaultConfig()
	a.Preset = "phew-blue"
	b := a
	b.Preset = "mono"

	if info.SigWith(a) == info.SigWith(b) {
		t.Error("changing preset must change the signature or --watch will skip the re-render")
	}
}

func TestSigWithChangesWhenLayoutChanges(t *testing.T) {
	info := DemoInfo()
	a := DefaultConfig()
	a.Layout = Layout{Corner: "bottom-right", Rows: []string{"os"}}
	b := a
	b.Layout = Layout{Corner: "top-left", Rows: []string{"os"}}
	c := a
	c.Layout = Layout{Corner: "bottom-right", Rows: []string{"os", "wan"}}

	if info.SigWith(a) == info.SigWith(b) {
		t.Error("corner change not reflected in the signature")
	}
	if info.SigWith(a) == info.SigWith(c) {
		t.Error("row change not reflected in the signature")
	}
}

func TestSigWithStableForSameInputs(t *testing.T) {
	info := DemoInfo()
	cfg := DefaultConfig()
	if info.SigWith(cfg) != info.SigWith(cfg) {
		t.Error("signature is not stable")
	}
}
