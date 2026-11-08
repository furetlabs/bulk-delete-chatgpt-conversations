package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/arush-sal/bulk-delete-chatgpt-conversations/internal/chatgpt"
)

func TestUpdateActionFinishedReturnsToSelectionAfterDelete(t *testing.T) {
	model := Model{
		phase:        phaseRunning,
		actionCursor: int(actionDelete),
		conversations: []chatgpt.Conversation{
			{ID: "1", Title: "alpha"},
			{ID: "2", Title: "beta"},
			{ID: "3", Title: "gamma"},
		},
		selected: map[string]struct{}{
			"1": {},
			"2": {},
		},
	}
	model.applyFilterAndSort()

	gotModel, cmd := model.Update(actionFinishedMsg{results: []actionResult{
		{id: "1", label: "alpha"},
		{id: "2", label: "beta"},
	}})
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", gotModel)
	}
	if cmd != nil {
		t.Fatalf("Update() cmd = non-nil, want nil")
	}
	if got.phase != phaseSelect {
		t.Fatalf("phase after delete completion = %v, want %v", got.phase, phaseSelect)
	}
	if len(got.conversations) != 1 || got.conversations[0].ID != "3" {
		t.Fatalf("remaining conversations = %+v, want only ID 3", got.conversations)
	}
	if len(got.selected) != 0 {
		t.Fatalf("selected after delete completion = %v, want empty", got.selected)
	}
}

func TestUpdateActionFinishedReturnsToSelectionAfterArchiveWithFailures(t *testing.T) {
	model := Model{
		phase:        phaseRunning,
		actionCursor: int(actionArchive),
		conversations: []chatgpt.Conversation{
			{ID: "1", Title: "alpha"},
			{ID: "2", Title: "beta"},
			{ID: "3", Title: "gamma"},
		},
		selected: map[string]struct{}{
			"1": {},
			"2": {},
		},
	}
	model.applyFilterAndSort()

	gotModel, cmd := model.Update(actionFinishedMsg{results: []actionResult{
		{id: "1", label: "alpha"},
		{id: "2", label: "beta", err: errTestActionFailed{}},
	}})
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", gotModel)
	}
	if cmd != nil {
		t.Fatalf("Update() cmd = non-nil, want nil")
	}
	if got.phase != phaseSelect {
		t.Fatalf("phase after archive completion = %v, want %v", got.phase, phaseSelect)
	}
	if len(got.conversations) != 2 || got.conversations[0].ID != "2" || got.conversations[1].ID != "3" {
		t.Fatalf("remaining conversations = %+v, want IDs 2 and 3", got.conversations)
	}
	if _, ok := got.selected["2"]; !ok {
		t.Fatalf("failed archive should remain selected, got selected = %v", got.selected)
	}
	if _, ok := got.selected["1"]; ok {
		t.Fatalf("successful archive should be cleared from selection, got selected = %v", got.selected)
	}
}

func TestUpdatePhaseDoneQuitsOnQ(t *testing.T) {
	model := Model{phase: phaseDone}

	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("Update() cmd = nil, want quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdateSelectionPageDownKeepsCursorValidWhenFilterHasNoResults(t *testing.T) {
	model := Model{
		phase:      phaseSelect,
		height:     24,
		filterText: "missing",
		conversations: []chatgpt.Conversation{
			{ID: "1", Title: "alpha"},
			{ID: "2", Title: "beta"},
		},
		selected: make(map[string]struct{}),
	}
	model.applyFilterAndSort()

	gotModel, _ := model.updateSelection(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("updateSelection() returned %T, want tui.Model", gotModel)
	}

	if got.cursor != 0 {
		t.Fatalf("cursor after pgdown with empty filter = %d, want 0", got.cursor)
	}
	if len(got.filtered) != 0 {
		t.Fatalf("filtered count = %d, want 0", len(got.filtered))
	}
}

func TestUpdateSelectionPageDownUsesRenderedSelectionHeight(t *testing.T) {
	model := Model{
		phase:      phaseSelect,
		width:      80,
		height:     24,
		filterText: "chat",
		selected:   make(map[string]struct{}),
	}
	for i := 0; i < 20; i++ {
		model.conversations = append(model.conversations, chatgpt.Conversation{
			ID:    string(rune('a' + i)),
			Title: "chat",
		})
	}
	model.applyFilterAndSort()

	gotModel, _ := model.updateSelection(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("updateSelection() returned %T, want tui.Model", gotModel)
	}

	if got.cursor != 8 {
		t.Fatalf("cursor after pgdown with active filter = %d, want 8", got.cursor)
	}
}

func TestUpdateSelectionPageUpUsesRenderedSelectionHeight(t *testing.T) {
	model := Model{
		phase:      phaseSelect,
		width:      80,
		height:     24,
		filterText: "chat",
		cursor:     18,
		selected:   make(map[string]struct{}),
	}
	for i := 0; i < 20; i++ {
		model.conversations = append(model.conversations, chatgpt.Conversation{
			ID:    string(rune('a' + i)),
			Title: "chat",
		})
	}
	model.applyFilterAndSort()
	model.cursor = 18

	gotModel, _ := model.updateSelection(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("updateSelection() returned %T, want tui.Model", gotModel)
	}

	if got.cursor != 10 {
		t.Fatalf("cursor after pgup with active filter = %d, want 10", got.cursor)
	}
}

func TestToggleCurrentIgnoresStaleNegativeCursor(t *testing.T) {
	model := Model{
		cursor: -1,
		filtered: []chatgpt.Conversation{
			{ID: "1", Title: "alpha"},
		},
		selected: make(map[string]struct{}),
	}

	model.toggleCurrent()

	if _, ok := model.selected["1"]; !ok {
		t.Fatal("toggleCurrent() did not recover from a stale negative cursor")
	}
	if model.cursor != 0 {
		t.Fatalf("cursor after toggleCurrent() = %d, want 0", model.cursor)
	}
}

func TestLoadResultUsesCacheThenRefreshesVisibleList(t *testing.T) {
	model := Model{
		phase:    phaseLoading,
		selected: make(map[string]struct{}),
	}

	afterCache, _ := model.Update(loadResultMsg{
		conversations: []chatgpt.Conversation{
			{ID: "cached", Title: "Cached"},
		},
	})

	cachedModel, ok := afterCache.(Model)
	if !ok {
		t.Fatalf("after cache update type = %T, want tui.Model", afterCache)
	}
	if cachedModel.phase != phaseSelect {
		t.Fatalf("phase after cache load = %v, want %v", cachedModel.phase, phaseSelect)
	}
	if len(cachedModel.conversations) != 1 || cachedModel.conversations[0].ID != "cached" {
		t.Fatalf("cached conversations = %#v, want cached conversation", cachedModel.conversations)
	}

	afterRefresh, _ := cachedModel.Update(loadResultMsg{
		refresh: true,
		conversations: []chatgpt.Conversation{
			{ID: "fresh-1", Title: "Fresh 1"},
			{ID: "fresh-2", Title: "Fresh 2"},
		},
	})

	refreshedModel, ok := afterRefresh.(Model)
	if !ok {
		t.Fatalf("after refresh update type = %T, want tui.Model", afterRefresh)
	}
	if got := len(refreshedModel.conversations); got != 2 {
		t.Fatalf("conversation count after refresh = %d, want 2", got)
	}
	if refreshedModel.conversations[0].ID != "fresh-1" || refreshedModel.conversations[1].ID != "fresh-2" {
		t.Fatalf("refreshed conversations = %#v, want refreshed list", refreshedModel.conversations)
	}
}

func TestEmptyCacheLoadWaitsForRefresh(t *testing.T) {
	model := Model{
		phase:    phaseLoading,
		selected: make(map[string]struct{}),
	}

	gotModel, _ := model.Update(loadResultMsg{})
	got, ok := gotModel.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", gotModel)
	}

	if got.phase != phaseLoading {
		t.Fatalf("phase after empty cache load = %v, want %v", got.phase, phaseLoading)
	}
	if len(got.conversations) != 0 {
		t.Fatalf("conversations after empty cache load = %#v, want empty", got.conversations)
	}
}

type errTestActionFailed struct{}

func (errTestActionFailed) Error() string {
	return "action failed"
}

func TestFilterChangeClearsHiddenSelection(t *testing.T) {
	m := Model{phase: phaseSelect, filtering: true, selected: map[string]struct{}{"a": {}}, conversations: []chatgpt.Conversation{{ID: "a", Title: "alpha"}}}
	next, _ := m.updateSelection(tea.KeyPressMsg(tea.Key{Text: "b", Code: 'b'}))
	if len(next.(Model).selected) != 0 {
		t.Fatal("selection remains after filter change")
	}
}

func TestRefreshCannotInterruptRunningBatch(t *testing.T) {
	m := Model{phase: phaseRunning, selected: map[string]struct{}{"a": {}}, conversations: []chatgpt.Conversation{{ID: "a", Title: "A"}}}
	next, _ := m.Update(loadResultMsg{refresh: true, conversations: []chatgpt.Conversation{{ID: "b", Title: "B"}}})
	got := next.(Model)
	if got.phase != phaseRunning || got.conversations[0].ID != "a" {
		t.Fatal("refresh interrupted batch")
	}
}

func TestSelectAllUsesVisibleIDsNotCounts(t *testing.T) {
	m := Model{phase: phaseSelect, selected: map[string]struct{}{"a": {}}, conversations: []chatgpt.Conversation{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}, filtered: []chatgpt.Conversation{{ID: "b", Title: "B"}}}
	next, _ := m.updateSelection(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	if _, ok := next.(Model).selected["b"]; !ok {
		t.Fatal("visible conversation not selected")
	}
}

func TestCancelledBatchStopsBeforeNextConversation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	results := runBatch(ctx, []chatgpt.Conversation{{ID: "a"}, {ID: "b"}}, func(ctx context.Context, id string) error { calls++; cancel(); return nil })
	if calls != 1 || len(results) != 1 {
		t.Fatalf("calls=%d results=%d", calls, len(results))
	}
}
