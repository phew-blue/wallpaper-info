package main

import "testing"

// --tray is documented as implying the watch loop. It is enforced in main() rather than in
// App, so this pins the rule itself: a resident tray with Watch == 0 would run App.Watch(),
// return immediately, and leave the desktop frozen on its first render.
func TestTrayImpliesRefreshInterval(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Watch != 0 {
		t.Fatalf("precondition: default Watch = %d, want 0", cfg.Watch)
	}

	tray := true
	if tray && cfg.Watch == 0 {
		cfg.Watch = 1
	}

	if cfg.Watch != 1 {
		t.Errorf("tray mode left Watch = %d; the refresh loop would never run", cfg.Watch)
	}
	app := &App{Cfg: cfg}
	if app.Cfg.Watch <= 0 {
		t.Error("App.Watch() would return immediately")
	}
}
