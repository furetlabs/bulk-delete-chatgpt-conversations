//go:build ci

package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/arush-sal/bulk-delete-chatgpt-conversations/internal/chatgpt"
)

func walk(o fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	visit(o)
	switch v := o.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			walk(child, visit)
		}
	case *container.Scroll:
		walk(v.Content, visit)
	case *widget.PopUp:
		walk(v.Content, visit)
	}
}
func findButton(root fyne.CanvasObject, label string) *widget.Button {
	var found *widget.Button
	walk(root, func(o fyne.CanvasObject) {
		if b, ok := o.(*widget.Button); ok && b.Text == label {
			found = b
		}
	})
	return found
}
func findCheck(root fyne.CanvasObject, label string) *widget.Check {
	var found *widget.Check
	walk(root, func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok && c.Text == label {
			found = c
		}
	})
	return found
}
func TestSimulationSearchSelectionProtectionAndRestart(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	w := newWindow(a)
	var search *widget.Entry
	walk(w.Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && e.PlaceHolder != "" {
			search = e
		}
	})
	if search == nil {
		t.Fatal("missing search")
	}
	search.SetText("projet")
	if findCheck(w.Content(), "Exemple : idées") != nil || findCheck(w.Content(), "Exemple : projet") == nil {
		t.Fatal("filter failed")
	}
	item := findCheck(w.Content(), "Exemple : projet")
	test.Tap(item)
	test.Tap(findButton(w.Content(), "Supprimer"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("preview absent")
	}
	var preview string
	walk(popup, func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && e.MultiLine {
			preview = e.Text
		}
	})
	if !strings.Contains(preview, "Exemple : projet") || strings.Contains(preview, "Exemple : idées") {
		t.Fatalf("unexpected preview %q", preview)
	}
	if b := findButton(popup, "Annuler"); b != nil {
		test.Tap(b)
	} else {
		t.Fatal("cancel unavailable")
	}
	var protect *widget.Check
	walk(w.Content(), func(o fyne.CanvasObject) {
		if c, ok := o.(*widget.Check); ok && c.Text == "Protégée" {
			protect = c
		}
	})
	if protect == nil {
		t.Fatal("protection missing")
	}
	test.Tap(protect)
	if c := findCheck(w.Content(), "Exemple : projet"); c == nil || !c.Disabled() {
		t.Fatal("protected item is selectable")
	}
	search.SetText("")
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	popup = w.Canvas().Overlays().Top()
	var allPreview string
	walk(popup, func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && e.MultiLine {
			allPreview = e.Text
		}
	})
	if strings.Contains(allPreview, "Exemple : projet") || !strings.Contains(allPreview, "Exemple : idées") {
		t.Fatalf("protected item leaked into preview: %q", allPreview)
	}
	test.Tap(findButton(popup, "Annuler"))
	w.Close()
	w2 := newWindow(a)
	defer w2.Close()
	c := findCheck(w2.Content(), "Exemple : projet")
	if c == nil || !c.Disabled() {
		t.Fatal("protection did not persist after restart")
	}
}

func TestSimulatedBatchCanBeCancelled(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	started := make(chan struct{}, 1)
	stopped := make(chan struct{}, 1)
	done := make(chan struct{})
	w := newWindowWithSimulator(a, func(ctx context.Context, id string) error {
		started <- struct{}{}
		<-ctx.Done()
		stopped <- struct{}{}
		return ctx.Err()
	}, func() { close(done) })
	defer w.Close()
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("preview absent")
	}
	confirm := findButton(popup, "Confirmer")
	if confirm == nil {
		t.Fatal("missing confirmation")
	}
	test.Tap(confirm)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("simulation never started")
	}
	test.Tap(findButton(w.Content(), "Annuler le lot"))
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("simulation did not stop")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("UI did not finish")
	}
}

func TestSimulationRejectsConcurrentBatch(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	done := make(chan struct{})
	w := newWindowWithSimulator(a, func(ctx context.Context, id string) error {
		started <- struct{}{}
		<-ctx.Done()
		finished <- struct{}{}
		return ctx.Err()
	}, func() { close(done) })
	defer w.Close()
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	popup := w.Canvas().Overlays().Top()
	test.Tap(findButton(popup, "Confirmer"))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("batch not started")
	}
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	if w.Canvas().Overlays().Top() != nil {
		t.Error("second batch allowed during first")
	}
	test.Tap(findButton(w.Content(), "Annuler le lot"))
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("batch not cancelled")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("UI did not finish")
	}
}

func TestSimulatedBatchCompletesAndShowsProgress(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	done := make(chan struct{})
	w := newWindowWithSimulator(a, func(context.Context, string) error { return nil }, func() { close(done) })
	defer w.Close()
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("missing preview")
	}
	test.Tap(findButton(popup, "Confirmer"))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("simulation did not finish")
	}
	var progress *widget.ProgressBar
	var summary string
	walk(w.Content(), func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.ProgressBar:
			progress = v
		case *widget.Label:
			if strings.Contains(v.Text, "Simulation terminée") {
				summary = v.Text
			}
		}
	})
	if progress == nil || progress.Value < 0.99 || !strings.Contains(summary, "3/3") {
		t.Fatalf("incorrect result: %v %q", progress, summary)
	}
	if findCheck(w.Content(), "Exemple : idées") != nil {
		t.Fatal("simulated removal not reflected")
	}
	test.Tap(findButton(w.Content(), "Simulation"))
	if findCheck(w.Content(), "Exemple : idées") == nil {
		t.Fatal("simulation reload failed")
	}
}

func TestWindowCloseCancelsRunningSimulation(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	started := make(chan struct{})
	done := make(chan struct{})
	w := newWindowWithSimulator(a, func(ctx context.Context, _ string) error { close(started); <-ctx.Done(); return ctx.Err() }, func() { close(done) })
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	test.Tap(findButton(w.Canvas().Overlays().Top(), "Confirmer"))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("simulation never started")
	}
	w.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("window close did not cancel simulation")
	}
}

func TestSimulationStopsAndDisplaysError(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	done := make(chan struct{})
	var calls atomic.Int32
	w := newWindowWithSimulator(a, func(context.Context, string) error { calls.Add(1); return errors.New("erreur fictive") }, func() { close(done) })
	defer w.Close()
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	test.Tap(findButton(w.Canvas().Overlays().Top(), "Confirmer"))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("simulation did not stop")
	}
	var summary string
	walk(w.Content(), func(o fyne.CanvasObject) {
		if l, ok := o.(*widget.Label); ok && strings.Contains(l.Text, "erreur fictive") {
			summary = l.Text
		}
	})
	if calls.Load() != 1 || summary == "" {
		t.Fatalf("missing error status or later action executed: %d, %q", calls.Load(), summary)
	}
}

func TestConfirmationRejectsNewlyProtectedTarget(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	called := make(chan struct{}, 3)
	done := make(chan struct{}, 1)
	w := newWindowWithSimulator(a, func(context.Context, string) error { called <- struct{}{}; return nil }, func() { done <- struct{}{} })
	defer w.Close()
	test.Tap(findButton(w.Content(), "Tout sauf protégées"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("missing preview")
	}
	if err := chatgpt.SetProtected("11111111-1111-4111-8111-111111111111", true); err != nil {
		t.Fatal(err)
	}
	test.Tap(findButton(popup, "Confirmer"))
	select {
	case <-called:
		<-done
		t.Fatal("started batch despite changed protection")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestChangingFilterClearsPreviousSelection(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	w := newWindow(a)
	defer w.Close()
	test.Tap(findCheck(w.Content(), "Exemple : idées"))
	var search *widget.Entry
	walk(w.Content(), func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && e.PlaceHolder != "" {
			search = e
		}
	})
	search.SetText("projet")
	test.Tap(findButton(w.Content(), "Supprimer"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("expected empty selection notice")
	}
	var hasPreview bool
	walk(popup, func(o fyne.CanvasObject) {
		if e, ok := o.(*widget.Entry); ok && e.MultiLine {
			hasPreview = true
		}
	})
	if hasPreview {
		t.Fatal("a hidden selection survived filtering")
	}
}

func TestSimulatedArchiveOfSelectedConversation(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	a := test.NewTempApp(t)
	done := make(chan struct{})
	w := newWindowWithSimulator(a, func(context.Context, string) error { return nil }, func() { close(done) })
	defer w.Close()
	test.Tap(findCheck(w.Content(), "Exemple : idées"))
	test.Tap(findButton(w.Content(), "Archiver"))
	popup := w.Canvas().Overlays().Top()
	if popup == nil {
		t.Fatal("archive preview missing")
	}
	test.Tap(findButton(popup, "Confirmer"))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("archive did not complete")
	}
	if findCheck(w.Content(), "Exemple : idées") != nil || findCheck(w.Content(), "Exemple : projet") == nil {
		t.Fatal("wrong simulated archive result")
	}
}
