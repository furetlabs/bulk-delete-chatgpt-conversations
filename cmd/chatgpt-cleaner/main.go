// ChatGPT Cleaner provides a local desktop interface. Simulation is the default.
package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/arush-sal/bulk-delete-chatgpt-conversations/internal/chatgpt"
	"github.com/arush-sal/bulk-delete-chatgpt-conversations/internal/cleaner"
)

func main() {
	a := app.NewWithID("org.furetlabs.chatgptcleaner")
	w := newWindow(a)
	w.ShowAndRun()
}

func newWindow(a fyne.App) fyne.Window {
	return newWindowWithSimulator(a, func(ctx context.Context, _ string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
			return nil
		}
	}, nil)
}

func newWindowWithSimulator(a fyne.App, simulate func(context.Context, string) error, finished func()) fyne.Window {
	w := a.NewWindow("ChatGPT Cleaner")
	w.Resize(fyne.NewSize(900, 650))
	if err := chatgpt.InitializeProtections(); err != nil {
		dialog.ShowError(err, w)
		return w
	}
	var items []cleaner.Item
	selected := map[string]bool{}
	var cancel context.CancelFunc
	var loadCancel context.CancelFunc
	var closed atomic.Bool
	var client *chatgpt.Client
	simulation := true
	var busy atomic.Bool
	status := widget.NewLabel("Simulation : aucune connexion à ChatGPT")
	progress := widget.NewProgressBar()
	search := widget.NewEntry()
	search.SetPlaceHolder("Rechercher une conversation")
	rows := container.NewVBox()
	var refresh func()
	refresh = func() {
		rows.RemoveAll()
		for _, v := range items {
			item := v
			if !strings.Contains(strings.ToLower(item.Title), strings.ToLower(search.Text)) {
				continue
			}
			check := widget.NewCheck(item.Title, func(b bool) { selected[item.ID] = b })
			check.SetChecked(selected[item.ID])
			if item.Protected {
				check.Disable()
			}
			protect := widget.NewCheck("Protégée", nil)
			protect.SetChecked(item.Protected)
			protect.OnChanged = func(b bool) {
				if busy.Load() {
					refresh()
					return
				}
				if err := chatgpt.SetProtected(item.ID, b); err != nil {
					dialog.ShowError(err, w)
					return
				}
				for i := range items {
					if items[i].ID == item.ID {
						items[i].Protected = b
					}
				}
				delete(selected, item.ID)
				refresh()
			}
			rows.Add(container.NewBorder(nil, nil, nil, protect, check))
		}
		rows.Refresh()
	}
	search.OnChanged = func(string) { selected = map[string]bool{}; refresh() }
	loadDemo := func() {
		if busy.Load() {
			return
		}
		simulation = true
		selected = map[string]bool{}
		items = []cleaner.Item{{ID: "11111111-1111-4111-8111-111111111111", Title: "Exemple : idées"}, {ID: "22222222-2222-4222-8222-222222222222", Title: "Exemple : projet"}, {ID: "33333333-3333-4333-8333-333333333333", Title: "Exemple : notes"}}
		for i := range items {
			p, e := chatgpt.IsProtected(items[i].ID)
			if e != nil {
				dialog.ShowError(e, w)
				return
			}
			items[i].Protected = p
		}
		status.SetText("Simulation : données fictives")
		refresh()
	}
	loadDemo()
	loadReal := func() {
		dialog.ShowConfirm("Connexion", "Ouvrir le navigateur et charger les conversations de votre compte ?", func(ok bool) {
			if !ok || !busy.CompareAndSwap(false, true) {
				return
			}
			ctx, stop := context.WithTimeout(context.Background(), 3*time.Minute)
			loadCancel = stop
			go func() {
				defer stop()
				finish := func(fn func()) {
					fyne.Do(func() {
						defer busy.Store(false)
						if !closed.Load() {
							fn()
						}
					})
				}
				auth, e := chatgpt.ResolveAuth()
				if e != nil {
					finish(func() { dialog.ShowError(e, w) })
					return
				}
				c, e := chatgpt.New(chatgpt.Config{SessionToken: auth.State.SessionToken, CSRFToken: auth.State.CSRFToken, AccessToken: auth.State.AccessToken, AllowLogin: true})
				if e != nil {
					finish(func() { dialog.ShowError(e, w) })
					return
				}
				list, e := c.ListConversations(ctx, 100)
				if e != nil {
					c.Close()
					finish(func() { dialog.ShowError(e, w) })
					return
				}
				fresh := make([]cleaner.Item, 0, len(list))
				for _, v := range list {
					p, e := chatgpt.IsProtected(v.ID)
					if e != nil {
						c.Close()
						finish(func() { dialog.ShowError(e, w) })
						return
					}
					fresh = append(fresh, cleaner.Item{ID: v.ID, Title: v.Title, Protected: p})
				}
				fyne.Do(func() {
					defer busy.Store(false)
					if closed.Load() {
						c.Close()
						return
					}
					if client != nil {
						client.Close()
					}
					client = c
					items = fresh
					selected = map[string]bool{}
					simulation = false
					status.SetText(fmt.Sprintf("Compte chargé : %d conversations", len(items)))
					refresh()
				})
			}()
		}, w)
	}
	operation := func(action string, all bool) {
		if busy.Load() {
			status.SetText("Une opération est déjà en cours")
			return
		}
		if !simulation {
			dialog.ShowInformation("Action indisponible", "Les opérations sur un compte réel ne sont pas activées : leurs garanties restent à valider.", w)
			return
		}
		targets := cleaner.Preview(items, selected, all)
		if len(targets) == 0 {
			dialog.ShowInformation("Aucune cible", "Aucune conversation non protégée sélectionnée.", w)
			return
		}
		var b strings.Builder
		for _, v := range targets {
			fmt.Fprintf(&b, "• %s [%s]\n", v.Title, v.ID)
		}
		review := widget.NewMultiLineEntry()
		review.SetText(b.String())
		review.Disable()
		content := container.NewBorder(widget.NewLabel(fmt.Sprintf("%s : %d conversations. Vérifiez la liste complète.", action, len(targets))), nil, nil, nil, container.NewScroll(review))
		d := dialog.NewCustomConfirm("Prévisualisation", "Confirmer", "Annuler", content, func(ok bool) {
			if !ok || !busy.CompareAndSwap(false, true) {
				return
			}
			for _, target := range targets {
				protected, err := chatgpt.IsProtected(target.ID)
				if err != nil || protected {
					busy.Store(false)
					dialog.ShowInformation("Liste modifiée", "Les protections ont changé ou ne sont pas disponibles. Vérifiez à nouveau la sélection.", w)
					return
				}
			}
			ctx, stop := context.WithCancel(context.Background())
			activeSimulation := simulation
			activeClient := client
			cancel = stop
			progress.SetValue(0)
			go func() {
				results := cleaner.Run(ctx, targets, func(ctx context.Context, id string) error {
					if activeSimulation {
						protected, err := chatgpt.IsProtectedContext(ctx, id)
						if err != nil {
							return err
						}
						if protected {
							return fmt.Errorf("conversation protégée")
						}
						return simulate(ctx, id)
					}
					if activeClient == nil {
						return fmt.Errorf("aucune session")
					}
					if action == "Supprimer" {
						return activeClient.DeleteConversation(ctx, id)
					}
					return activeClient.ArchiveConversation(ctx, id)
				}, func(r cleaner.Result) {
					fyne.Do(func() {
						if closed.Load() {
							return
						}
						progress.SetValue(progress.Value + 1/float64(len(targets)))
						if r.Err != nil {
							status.SetText("Erreur : " + r.Err.Error())
						}
					})
				})
				fyne.Do(func() {
					if closed.Load() {
						busy.Store(false)
						if finished != nil {
							finished()
						}
						return
					}
					busy.Store(false)
					successful := make(map[string]bool)
					for _, result := range results {
						if result.Err == nil {
							successful[result.Item.ID] = true
						}
					}
					remaining := items[:0]
					for _, item := range items {
						if !successful[item.ID] {
							remaining = append(remaining, item)
						}
					}
					items = remaining
					succeeded := 0
					for _, r := range results {
						if r.Err == nil {
							succeeded++
						}
					}
					if ctx.Err() != nil {
						status.SetText(fmt.Sprintf("Annulé après %d/%d succès", succeeded, len(targets)))
					} else if len(results) > 0 && results[len(results)-1].Err != nil {
						status.SetText(fmt.Sprintf("Erreur après %d succès : %v", succeeded, results[len(results)-1].Err))
					} else {
						status.SetText(fmt.Sprintf("Simulation terminée : %d/%d succès", succeeded, len(targets)))
					}
					selected = map[string]bool{}
					refresh()
					if finished != nil {
						finished()
					}
				})
			}()
		}, w)
		d.Resize(fyne.NewSize(700, 500))
		d.Show()
	}
	buttons := container.NewHBox(widget.NewButton("Simulation", loadDemo), widget.NewButton("Charger mon compte", loadReal), widget.NewButton("Archiver", func() { operation("Archiver", false) }), widget.NewButton("Supprimer", func() { operation("Supprimer", false) }), widget.NewButton("Tout sauf protégées", func() { operation("Supprimer", true) }), widget.NewButton("Annuler le lot", func() {
		if cancel != nil {
			cancel()
		}
	}))
	w.SetContent(container.NewBorder(container.NewVBox(status, search, buttons), progress, nil, nil, container.NewScroll(rows)))
	w.SetOnClosed(func() {
		closed.Store(true)
		if loadCancel != nil {
			loadCancel()
		}
		if cancel != nil {
			cancel()
		}
		if client != nil {
			client.Close()
		}
	})
	return w
}
