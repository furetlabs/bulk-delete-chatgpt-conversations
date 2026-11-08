# Plan de sécurisation et de fiabilisation

> Pour une exécution future par agent : utiliser `superpowers:executing-plans` tâche par tâche après validation explicite de ce document. Aucune exécution autorisée par la seule présence du plan.

**Objectif :** conserver une application Go en terminal, compatible Apple Silicon, dont la lecture est vérifiable et dont aucune mutation ne peut partir sans autorisation et confirmation explicites.

**Architecture proposée :** conserver CLI/TUI et modèles ; isoler l’authentification, le transport interne et la politique d’exécution des lots. Le mode par défaut est lecture seule. L’intégration privée reste remplaçable et désactivable si le contrat distant devient incompatible.

**Stack :** Go dans une version maintenue à sélectionner après contrôle des avis de sécurité ; Cobra, Bubble Tea, chromedp conservés sous réserve du scan. Bibliothèque standard pour HTTP, fichiers et orchestration. Keychain pour les secrets persistants macOS, avec choix de l’adaptateur soumis à revue avant ajout d’une dépendance ou de CGO.

**Spécification et preuves :** [AUDIT.md](AUDIT.md), commit `365ea598d01504ecb4279f10976078f92f6a5756`. Ce plan est une proposition à valider, pas une architecture déjà approuvée.

## Contraintes globales

- Ne jamais supprimer de conversation pendant cet audit ni son éventuelle validation en lecture seule.
- Ne jamais transmettre de jeton d’authentification à un outil d’audit, un log, un tiers ou un rapport. Une application utilisant le backend transmet nécessairement des credentials au service d’origine : ce futur comportement demande un périmètre explicitement autorisé, il n’est pas autorisé par l’audit.
- Ne pas contourner les mécanismes de sécurité ; laisser login, MFA et challenges à l’utilisateur, arrêter si accès refusé.
- Ne pas exécuter de code suspect ni d’opération destructive ; les tests de mutation utilisent exclusivement des simulateurs et des données factices.
- Ne pas supposer qu’une API fonctionne sans vérification ; distinguer build, simulation, lecture distante et mutation distante.
- Ne pas développer avant validation. Une validation de ce plan n’autorise ni suppression/archivage réel ni collecte/export de secrets.
- Pas de nouvelle GUI, de serveur local exposé, de stockage distant ni d’auto-update dans le périmètre proposé.

## Choix d’approche et décision attendue

| Option | Effort / compromis | Avis |
| --- | --- | --- |
| A — adapter ponctuellement | Corriger les symptômes dans les fichiers actuels ; rapide mais conserve couplages et courses d’état. | Insuffisant pour l’exécution de lots sensibles. |
| B — réécrire les composants sensibles | Conserver Go/CLI/TUI ; remplacer session, stockage et coordination des lots, durcir l’adaptateur privé. | Recommandé. Permet une livraison initiale en lecture seule. |
| C — repartir de zéro | Reconstruire UI et backend, coût accru ; les incertitudes des API internes demeurent. | Non justifié par la taille et l’état du projet. |

Choix proposé : **B**, sans figer maintenant une nouvelle technologie graphique. Si « aucun jeton extrait du navigateur, même dans le produit final » devient une exigence durable, réexaminer l’intégration : session confinée au navigateur avec actions officielles éventuellement assistées, ou simple analyse d’un export utilisateur. Aucune API publique équivalente pour administrer l’historique ChatGPT n’a été établie par l’audit.

## Architecture cible minimale

```text
Cobra / Bubble Tea
       │
       ├─ AuthManager ── BrowserSession (CDP local et borné)
       │              └─ SecretStore (mémoire / Keychain)
       │
       └─ BatchRunner ── ConversationBackend (adaptateur interne)
                        └─ cache partitionné par identité/espace
```

La TUI reçoit des snapshots immuables et des événements de progression, sans lire directement des secrets. Le backend garde les credentials privés, valide l’origine HTTPS, la taille et le schéma des réponses. La politique de mutation appartient au runner et au backend, pour qu’un appel direct ne puisse contourner le mode lecture seule. Les IDs confirmés sont figés, dédupliqués et liés à une identité et à une génération de liste fraîche.

Types proposés, à introduire dans `internal/chatgpt/types.go` : `AccountScope{UserID, WorkspaceID string}`, `ConversationSnapshot{Scope AccountScope, Generation uint64, FetchedAt time.Time, Items []Conversation, Fresh bool}`, `OperationResult{ID string, State OperationState, Err error}`. `OperationState` distingue `NotStarted`, `Succeeded`, `Failed`, `Unknown` ; une erreur disque après un succès distant ne doit pas devenir un échec distant. Si l’identité/espace ne peut être établi de façon fiable, conserver la lecture seule.

Interfaces limitées : `ConversationBackend.List(ctx context.Context) (ConversationSnapshot, error)` et `ConversationBackend.Apply(ctx context.Context, scope AccountScope, id string, action Action) OperationResult` ; `SecretStore.Load(ctx context.Context) (AuthState, error)`, `Save(ctx context.Context, state AuthState) error`, `Delete(ctx context.Context) error`. Ces types sont internes ; aucun jeton dans les événements UI ou les journaux.

## Points de revue transversaux

1. Identité/espace changé pendant une opération : zéro nouvelle mutation avant nouvelle validation (tâches 2–4).
2. Messages cache/réseau reçus en retard, y compris pendant un lot : lot inchangé et aucune réouverture de sélection (tâche 4).
3. Réponse serveur incompatible, vide ou trop volumineuse : erreur explicite, jamais « zéro conversation » ou succès supposé (tâche 3).
4. Arrêt utilisateur ou réponse perdue après envoi : pas de nouvelle cible, état `Unknown` distinct d’un échec (tâche 4).
5. Données contrôlées par serveur/fichier : aucun secret ni séquence de contrôle dans les rendus ; aucun suivi de symlink sensible (tâches 1–2).

## Tâches proposées — ne pas exécuter avant validation

Chaque tâche commence par des tests de régression échouant sur le défaut, puis une correction minimale, la vérification et une revue du diff. Les commandes ci-dessous sont des contrôles futurs, pas des résultats acquis. Commits locaux seulement si demandés dans la phase de développement ; aucune publication implicite.

### 1. Supprimer les fuites et établir la barrière de lecture seule

**Fichiers :** modifier `internal/chatgpt/client.go`, `auth_state.go`, `internal/tui/model.go`, `cmd/chatgpt-bulk/main.go` et leurs tests ; créer `internal/chatgpt/policy.go` et `policy_test.go`.

**Interface :** `MutationPolicy{Enabled bool}` ajouté à `Config`, valeur zéro = interdiction ; `ErrReadOnly` commun. Les méthodes publiques Archive/Delete vérifient la politique avant toute initialisation réseau.

- [ ] Ajouter `TestReadOnlyBlocksArchiveAndDeleteBeforeTransport` : transport espion, 0 requête et `ErrReadOnly`, même avec credentials présents. Les tests existants de mutation activent explicitement une politique factice et restent locaux.
- [ ] Ajouter `TestAuthLogsContainNoSecrets` avec secrets synthétiques longs/courts, session JSON, cookies, erreurs et DEBUG ; aucun secret ni fragment volontaire dans logs, entêtes affichés ou vues.
- [ ] Ajouter `TestRemoteTextCannotInjectTerminalControls` : ESC, OSC, retours ligne et titres trompeurs nettoyés, Unicode utile préservé.
- [ ] Remplacer les logs de corps par méthode, statut et catégorie d’erreur ; retirer `SessionIDLabel`/`MaskToken` du rendu et nettoyer les textes externes à la frontière d’affichage.
- [ ] Garder la CLI/TUI en lecture seule par défaut ; toute activation future doit être explicite et rester soumise à la confirmation de lot.
- [ ] Vérifier `go test -count=1 ./internal/chatgpt ./internal/tui ./cmd/chatgpt-bulk` : tous les tests passent sans accès distant. Revoir le diff avant tâche suivante.

**Complexité :** moyenne, 1–2 jours. Couvre S01, S13, S15 et le préalable de sécurité des tâches suivantes.

### 2. Remplacer la gestion de session et protéger les données locales

**Fichiers :** extraire `internal/chatgpt/auth_manager.go`, `browser.go`, `secret_store.go`, `secret_store_darwin.go` ; modifier `auth_state.go`, `chrome_unix.go`, `conversation_cache.go`, `client.go`, CLI, harness et tests associés. Les fichiers nouveaux sont des frontières utiles, pas un framework générique.

**Interfaces :** `AuthManager.Authenticate(ctx context.Context, fresh bool) (AccountScope, error)` et `Close(ctx context.Context) error` ; conserver les secrets privés. Implémenter `SecretStore` décrit plus haut et un stockage mémoire pour les tests. Cache enveloppé avec `AccountScope`, date de dernier fetch réellement actualisée.

- [ ] Tests `TestAuthLifecycleReopensAfterRejectedSession`, `TestBrowserCommandsHonorCancellation`, `TestCloseStopsOwnedBrowserAndCleansAllProfiles` avec navigateur/CDP simulé ; vérifier les chemins erreur, timeout, rafraîchissements successifs et fermeture répétée.
- [ ] Tests `TestExistingTokenIsNotProofOfValidSession` et `TestFreshLoginDoesNotSilentlyReuseAnotherAccount` ; distinguer « état présent » et « session vérifiée » dans CLI/harness.
- [ ] Tests `TestSecretStoreRejectsSymlinkAndUnsafeExistingFile`, interruption d’écriture et permissions ; pas de fichier utilisateur réel. Écriture atomique dans un dossier contrôlé et refus des cibles non sûres.
- [ ] Tests `TestCacheIsAccountScoped`, changement d’espace, `logout` et session temporaire ; aucun titre ancien affiché à une nouvelle identité. Si l’identité manque, désactiver l’usage actionnable du cache.
- [ ] Imposer le débogage loopback, contrôler le processus/profil possédé, propager le contexte, remettre tous les pointeurs d’état à zéro à la fermeture et unifier la synchronisation des snapshots.
- [ ] Sur macOS, implémenter le stockage Keychain après revue du mécanisme retenu ; pas de secret passé dans des arguments de processus ou dans un log. Garder le mode mémoire par défaut et différer le stockage persistant sur une autre plateforme si aucune solution sûre n’est disponible.
- [ ] Ne pas migrer/supprimer silencieusement l’ancien JSON : migration explicite, écriture protégée vérifiée puis suppression autorisée séparément. Corriger chemins et promesses dans README/docs/.env.example.
- [ ] Corriger le harness : chemin binaire absolu, refus d’écraser un fichier explicite existant, fixtures uniquement temporaires ; ne pas le lancer contre un compte.
- [ ] Vérifier `go test -race -count=1 ./internal/chatgpt ./cmd/chatgpt-bulk ./cmd/chatgpt-bulk-auth-e2e`, puis tests natifs du store sur macOS. Résultat attendu : aucun race report et aucune fuite ; macOS requis pour valider Keychain.

**Complexité :** élevée, 3–5 jours, dépend du choix Keychain. Couvre S02, S03, S05, S08, S09, S11–S14.

### 3. Isoler et valider l’adaptateur des interfaces privées

**Fichiers :** créer `internal/chatgpt/types.go`, `backend.go`, `backend_test.go` ; déplacer HTTP/pagination/mutations depuis `client.go`. Adapter `client_test.go` et les appels UI au contrat.

**Interface :** `ConversationBackend` défini plus haut ; erreurs typées auth expirée, accès refusé, débit limité, contrat incompatible et résultat inconnu. Le constructeur prend une configuration d’origine fixe en production et un transport injectable pour les tests.

- [ ] Fixtures synthétiques pour `TestListRejectsMissingItemsAndRepeatedPages`, IDs dupliqués/vides, dates invalides, HTML 200, corps vide, 204 inattendu, réponse trop grande et limite de pages. Conserver une erreur explicite au lieu de remplacer un cache valide par une fausse liste vide.
- [ ] `TestTransportRejectsUnexpectedOriginAndRedirect`, URLs/IDs malformés, HTTPS obligatoire ; les tests vérifient qu’aucun credential ne quitte le transport de test autorisé.
- [ ] `TestPatchRequiresValidatedResponse` et `TestPatchPayloadMatchesAction` contre serveur local uniquement ; n’accepter que des réponses dont la sémantique a été établie. Tant que le contrat distant manque, garder les mutations désactivées.
- [ ] Traiter 401, 403, 429 et 5xx distinctement ; stopper le lot sur perte d’identité ou refus d’accès. Pas de retry aveugle de mutation dont l’issue est inconnue ; le backoff de lecture respecte `Retry-After` et le contexte.
- [ ] Ajouter des tests de contrats versionnés sans données privées. Un contrat réellement observé ne peut être introduit qu’après autorisation distincte et anonymisation locale sans export de secrets.
- [ ] Vérifier `go test -count=1 ./internal/chatgpt` et `go vet ./...` ; pas d’appel réseau ChatGPT.

**Complexité :** moyenne à élevée, 2–3 jours hors clarification du service distant. Couvre S10 et les limites de récupération/API. Aucune abstraction multi-fournisseur ni endpoint arbitraire exposé à l’utilisateur.

### 4. Remplacer l’exécution des lots et sécuriser la sélection

**Fichiers :** créer `internal/chatgpt/batch.go`, `batch_test.go` ; modifier `internal/tui/model.go` et `model_test.go`.

**Interfaces :** `BatchPlan{Scope AccountScope, Generation uint64, Action Action, IDs []string}` ; `BatchRunner.Run(ctx context.Context, plan BatchPlan, emit func(OperationResult)) error`. Plan immuable après revue, lié au dernier snapshot vérifié. Le runner revalide scope/génération et politique avant tout appel backend.

- [ ] `TestFilterChangeCannotHideActionTargets` : par défaut, un changement de filtre efface la sélection avec indication visible ; « tout sélectionner » agit sur l’ensemble visible et compare les IDs. Préserver cette règle dans les tests Unicode.
- [ ] `TestStaleOrForeignSnapshotCannotCreateBatch`, `TestLateRefreshDoesNotReopenRunningBatch` et refresh pendant confirmation : une mise à jour invalide la confirmation avant exécution, jamais le lot en cours. Ignorer les générations obsolètes.
- [ ] `TestDeleteNeedsExplicitTypedConfirmation` : Entrée seule ne confirme jamais une suppression ; phrase proposée `SUPPRIMER <nombre>`, liée au compte et à la liste complète consultable. Archivage séparé, aucune confusion de libellé. Tests clavier répété, 0/1/plus de 8 cibles.
- [ ] `TestCancelStopsBeforeNextTarget`, `TestAuthFailureStopsRemainingTargets`, `TestDiskFailureDoesNotUndoRemoteSuccess`, `TestTimeoutAfterSendIsUnknown` sur backend factice. Le callback fournit un résultat à chaque cible ; pas de reprise automatique d’un état `Unknown`.
- [ ] Donner au modèle un contexte de lot annulable ; `q` annule, attend un bilan borné et n’affirme pas qu’une requête déjà envoyée a été annulée côté serveur. Afficher le bilan même si des conversations restent.
- [ ] Journal local minimal sans secrets ni titres par défaut ; ne pas présenter une liste de métadonnées comme sauvegarde des messages. Refuser une suppression réelle tant que sa sémantique n’est pas autorisée et vérifiée séparément.
- [ ] Vérifier `go test -race -count=1 ./internal/chatgpt ./internal/tui` : tous scénarios locaux passent, aucune mutation distante.

**Complexité :** élevée, 3–4 jours. Couvre S04–S07, S11 et les risques de pertes accidentelles.

### 5. Valider macOS et rendre la distribution traçable

**Fichiers :** `.github/workflows/ci.yml`, `release.yml`, workflows Pages, `.goreleaser.yaml`, `Makefile`, `scripts/install-via-bin.sh`, README, docs et liens du site.

- [ ] Ajouter une matrice Linux/macOS, un contrôle ARM64, tests race appropriés, scan `govulncheck` versionné et inventaire de dépendances. Fixer la version Go maintenue retenue après ce scan ; ne pas actualiser aveuglément toutes les dépendances.
- [ ] Tester nativement macOS Apple Silicon : terminal, chemins Application Support, Keychain, navigateur actuel, CDP exclusivement local, fermeture et profils sur timeout. Compte réel et connexion uniquement après autorisation spécifique ; tests de navigateur initialement sur pages locales.
- [ ] Corriger les cibles GOOS/GOARCH du Makefile et vérifier les formats d’artefacts sans les exécuter sur un hôte incompatible.
- [ ] Fixer actions par SHA, version GoReleaser et graphe de release ; séparer vérification et publication, exiger CI réussie pour diffuser. Vérifier les permissions Pages et les previews sans toucher à la production pendant les tests.
- [ ] Lier installation/releases au fork choisi ; vérifier empreintes et provenance du bootstrap, ou retirer ce bootstrap au profit d’un artefact versionné vérifiable. Ne pas exécuter un téléchargement non vérifié.
- [ ] Définir une diffusion macOS signée/notarisée avant large distribution ; aucun contournement Gatekeeper dans le parcours recommandé. Les credentials de signature seront fournis via stockage sécurisé, jamais dans ces documents.
- [ ] Vérifier `go test -count=1 ./...`, `go vet ./...`, build darwin/arm64, contrôle shell, scan actualisé et tests d’installeur dans un dossier temporaire avec téléchargements simulés. Publication réelle exclue de cette tâche sans autorisation explicite.

**Complexité :** moyenne, 2–3 jours hors signature et acquisition d’accès macOS. Couvre S16, les limites de plateforme et les incertitudes de dépendances.

## Estimation et séquencement

Ordre : 1 → 2 → 3 → 4 → 5. Ordre de grandeur **11–17 jours d’ingénierie**, pour une personne connaissant Go, incluant tests/revues locales ; incertitude élevée sur Keychain et contrat distant. Non inclus : nouvelle GUI, changement de politique ChatGPT, processus administratif de signature/notarisation, attente de matériel/compte et publication. Ce n’est pas une garantie de fonctionnement des interfaces internes.

Livrable intermédiaire utile après 1–3 : outil de lecture seule durci, fonctionnant contre simulateurs, sans risque de mutation depuis la TUI. Si aucune intégration légitime et stable n’est vérifiable, arrêter l’activation des actions et réorienter vers analyse d’export ou opérations manuelles dans l’interface officielle. Une réécriture générale ne résoudrait pas ce blocage.

## Conditions de validation et arrêt

Avant toute reprise de développement : validation explicite de l’option B et du périmètre ci-dessus. Ensuite, les tests simulés peuvent vérifier les mutations comme comportement de logiciel, sans autoriser une opération réelle.

Une validation distante ultérieure nécessite une autorisation distincte, des barrières bloquant les écritures, aucun log de secret et une session sous contrôle utilisateur. Elle vérifie seulement la lecture et l’authentification normale. Ne pas lancer le harness actuel par défaut ; ne pas fournir de token dans le chat ; ne pas automatiser de challenge. Un refus de service ou un contrat inconnu conduit à l’arrêt, pas à un contournement.

La suppression/archivage réels restent hors périmètre et interdits par la demande actuelle. Aucun critère de réussite du présent plan ne nécessite une suppression réelle.

**Recommandation finale : B — réécrire les composants sensibles, conserver le socle Go/TUI, et n’activer aucune mutation tant que ses garanties et son contrat ne sont pas établis. Développement en attente de validation.**
