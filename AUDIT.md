# Audit technique et sécurité

Date : 8 novembre 2026. Dépôt : `furetlabs/bulk-delete-chatgpt-conversations`, branche sélectionnée `master`, commit audité `365ea598d01504ecb4279f10976078f92f6a5756` (27 mars 2026).

## Synthèse

**Recommandation : B — réécrire certains composants, en conservant le projet Go et son interface terminal.** La base est compacte et compilable pour Apple Silicon, mais la gestion des secrets, le cycle de vie du navigateur et les garanties entourant les lots ne conviennent pas encore à des opérations sensibles sur un historique réel.

Priorités : retirer les données d’authentification des logs ; sécuriser leur stockage ; interdire les mutations depuis une vue périmée ou un autre compte ; figer le lot confirmé ; corriger annulation et renouvellement de session ; valider strictement le contrat des réponses.

**Compatibilité avec ChatGPT actuel : non établie.** Les routes observées dans le code sont des interfaces web internes. La compilation et les tests simulés ne prouvent ni leur disponibilité actuelle ni leur sémantique réelle. Aucun appel à ChatGPT n’a été effectué pendant cet audit.

## Périmètre et méthode

Inspection des sources Go de production et des tests, manifests et sommes de dépendances, Makefile, GoReleaser, workflows CI/release/Pages, documentation, script d’installation et site statique. Références de code ci-dessous relatives à ce commit. Les illustrations du site ne constituent pas une preuve de fonctionnement.

Seuls les deux documents demandés sont ajoutés. Aucun changement d’application, de test, de dépendance ou de configuration distante. Aucun login, navigateur authentifié, archivage ou suppression réelle ; aucun secret utilisateur lu ou transmis. Les tests relus utilisent des données factices, des fichiers temporaires et des serveurs HTTP locaux. Les routines de mutation sont testées uniquement contre ces simulateurs. Aucun installeur du dépôt, workflow de publication ou harness de connexion réelle n’a été lancé.

Ce rapport est un audit du checkout, pas une certification : absence d’analyse exhaustive de l’historique Git, des binaires publiés, de chaque source transitive, des paramètres GitHub effectifs et du service distant. Pas de scan CVE actualisé (`govulncheck` non exécuté) ; aucune conclusion « dépendances sans vulnérabilité » n’est donc formulée. Les constats statiques sont distingués des comportements validés localement et des risques restant à reproduire.

## Architecture

```text
Cobra CLI ──> Bubble Tea TUI ──> Client ChatGPT
                                  ├─ Chrome/Edge/Brave + CDP : connexion
                                  ├─ net/http + cookies + Bearer : opérations
                                  ├─ auth.json : secrets et identité
                                  └─ conversations.json : métadonnées et masquages
Site HTML/CSS/JS et installation/release : distribution séparée
```

| Composant | Responsabilité et limites |
| --- | --- |
| `cmd/chatgpt-bulk/main.go` | Commandes `login`, `auth status`, `logout`, configuration et lancement TUI. Orchestration directement couplée au client concret. |
| `internal/chatgpt/client.go` | Authentification, CDP, HTTP, pagination, mutations et cache dans un seul fichier de 1 144 lignes. Mutex sérialisant liste et mutations, mais pas tous les lecteurs d’état. |
| `internal/chatgpt/auth_state.go` | Lecture/écriture JSON des secrets, résolution du chemin, suppression locale et affichage partiel d’un jeton. |
| `internal/chatgpt/conversation_cache.go` | Cache local des métadonnées et registre des IDs masqués, sans partition de compte. |
| `internal/chatgpt/chrome_{unix,windows}.go` | Détection et lancement d’un navigateur détaché ; macOS utilise `open -na … --args`. |
| `internal/tui/model.go` | Machine à états chargement/sélection/action/confirmation/exécution ; 1 144 lignes mêlant affichage et orchestration. |
| `cmd/chatgpt-bulk-auth-e2e` | Harness manuel de connexion ; son nom E2E n’implique pas un test réel des opérations. |
| `internal/version` | Version injectée au build. |

Pas de serveur applicatif ni de base distante propre au projet. Aucune télémétrie applicative explicite repérée dans les sources de premier niveau. Cela ne couvre pas la télémétrie du navigateur ou de ChatGPT. Le site charge des polices Google, révélant au minimum des informations de connexion aux fournisseurs concernés lors de sa consultation.

## Dépendances et chaîne de livraison

Go minimal déclaré : **1.24.2**. Six dépendances directes, versions épinglées dans `go.mod`, sommes dans `go.sum` :

| Dépendance | Version | Rôle / exposition |
| --- | --- | --- |
| `charm.land/bubbletea/v2` | 2.0.2 | Boucle terminal et commandes asynchrones. |
| `github.com/charmbracelet/lipgloss` | 1.1.0 | Rendu ANSI ; ne dispense pas de nettoyer les chaînes non fiables. |
| `github.com/chromedp/chromedp` | 0.13.6 | Contrôle puissant du navigateur et accès aux cookies. |
| `github.com/chromedp/cdproto` | pseudo-version du 3 avril 2025 | Schéma CDP, susceptible de décalage avec un navigateur récent. |
| `github.com/joho/godotenv` | 1.5.1 | Chargement implicite de `.env` depuis le répertoire courant. |
| `github.com/spf13/cobra` | 1.10.2 | CLI et parsing des options. |

Transitives : bibliothèques Charm de terminal/Unicode, WebSocket Gobwas, JSON expérimental, `golang.org/x/sys` 0.40.0 et `x/sync` 0.19.0, entre autres. `go.sum` protège l’intégrité attendue des modules ; il ne prouve ni innocuité ni absence de CVE. Le navigateur et `bin` sont des dépendances d’exécution/distribution hors `go.mod`. Go 1.24.2 est la version utilisée pour la reproduction, pas une recommandation de sécurité à la date de l’audit : une version maintenue et ses correctifs doivent être vérifiés avant diffusion.

La CI exécute syntaxe shell, `go vet` et tests uniquement sur Ubuntu ; pas de matrice macOS, de test navigateur actuel, de scan de vulnérabilités ou de race detector. Les actions GitHub sont référencées par tags ; GoReleaser est demandé en `latest`, et son hook `go mod tidy` peut modifier le graphe pendant une release. Des checksums d’archives sont générés, sans configuration explicite de signature/notarisation macOS.

Le script `scripts/install-via-bin.sh` télécharge puis exécute un `bin` de la dernière release sans contrôle explicite d’empreinte/signature dans son bootstrap. La vérification éventuellement faite par `bin install` n’a pas été auditée. L’installation, les liens du site et `go install …@latest` ciblent toujours **l’amont `arush-sal`**, pas le fork `furetlabs`. Le module conservant son identité amont est acceptable techniquement ; distribuer le mauvais produit est un risque distinct. Le site affiche même une commande `curl` sans schéma explicite. Ne pas lancer ces commandes avant durcissement. La suppression de quarantaine proposée dans le README n’est pas une solution de distribution de confiance ; aucune manipulation Gatekeeper n’a été réalisée.

Licence racine : AGPL-3.0 ; préserver les obligations de licence et d’attribution lors d’une redistribution. Analyse juridique détaillée hors périmètre.

## Authentification et sessions

1. `ResolveAuth` charge un fichier JSON ; contrairement à `.env.example`/certains passages du README, il n’existe plus de fallback effectif vers `CHATGPT_SESSION_TOKEN` ou `CHATGPT_CSRF_TOKEN` dans ce chemin CLI.
2. Sans fichier, l’utilisateur choisit une session temporaire ou persistante. `login` peut réutiliser l’état déjà stocké, y compris en mode session-only : cela ne signifie pas connexion à un nouveau compte, ni effacement de l’ancien fichier.
3. Le client ouvre un profil navigateur temporaire via CDP. Si des cookies sont stockés, il réinjecte le cookie `__Secure-next-auth.session-token`, éventuellement découpé en fragments de 3 800 octets, et le cookie CSRF `__Host-next-auth.csrf-token`.
4. Le navigateur visite `https://chatgpt.com` ; un `fetch` avec `credentials: include` interroge `/api/auth/session` toutes les deux secondes, avec une limite nominale de deux minutes. L’utilisateur doit accomplir lui-même login/MFA/challenge. Aucun mécanisme explicite de résolution automatisée de challenge n’a été trouvé.
5. Le client extrait `accessToken`, l’adresse email et les cookies, initialise un client HTTP puis ferme le navigateur. Les requêtes suivantes comportent **Bearer et cookies**. Aucun OpenAI API key, OAuth applicatif officiel ou scope de permission granulaire n’est utilisé.
6. `AuthState` contient et `SaveAuthState` sérialise **session token, CSRF token, access token, email, date et source**. Le README affirmant que l’access token reste exclusivement en mémoire est incorrect (`client.go:575`, `auth_state.go:15`).

Les secrets persistants sont en clair, avec modes demandés 0700/0600 à la création. `os.WriteFile` ne corrige pas les permissions d’un fichier préexistant, suit les liens symboliques et ne fournit pas d’écriture atomique. Pas de Keychain, vérification du propriétaire, expiration explicite ou verrou interprocessus. Le mode temporaire ne garantit pas l’absence totale de traces : profil navigateur sur disque jusqu’au nettoyage, mémoire, swap, sauvegardes ou crash.

Sur macOS, `os.UserConfigDir()` conduit normalement à `~/Library/Application Support/chatgpt-bulk/auth.json`, **pas** au chemin Linux `~/.config` annoncé. Le cache partage le même répertoire ; les deux chemins peuvent être redirigés par variables. `logout` supprime seulement le fichier local : pas de révocation distante et pas d’effacement du cache de conversations.

`auth status` vérifie la présence et le décodage, pas la validité distante. `ensureReady` accepte immédiatement un access token non vide ; le harness de « réutilisation de session » peut donc annoncer un succès sans aucun contrôle réseau (`client.go:294`, harness `verifySavedAuthReuse`).

## Récupération des conversations

`ListConversations` (`client.go:183`) appelle `GET https://chatgpt.com/backend-api/conversations?offset=N&limit=100`. Il décode `items`, concatène les pages, incrémente l’offset de la taille demandée et s’arrête à une page vide. Une erreur 401/403 déclenche au maximum une tentative de renouvellement pour la requête concernée.

Les données chargées sont des **métadonnées**, notamment ID, titre et dates ; aucun téléchargement du contenu complet des messages ni export de sauvegarde n’est implémenté. Aucune preuve d’exhaustivité pour les archives, projets, espaces d’équipe ou conversations particulières. Le code ne sélectionne pas explicitement d’espace de travail.

Limites : pas de validation obligatoire de `items`, pas de déduplication d’IDs, pas de garde contre des pages répétées, pas de bornage de la réponse HTTP ; une réponse JSON `{}` est acceptée comme une liste vide. La pagination par offset peut manquer ou répéter des entrées si la liste évolue. Une page plus courte que `limit` n’est pas interprétée comme fin, ce qui ajoute une requête mais n’est pas en soi un défaut d’exhaustivité. Le cache conserve des IDs masqués indéfiniment et peut cacher une conversation rétablie dans l’interface officielle.

## Archivage, suppression et garanties réelles

| Intention | Appel codé | Validation actuelle |
| --- | --- | --- |
| Archiver | `PATCH /backend-api/conversation/{id}`, JSON `{"is_archived":true}` | N’importe quel statut 2xx est accepté. |
| Supprimer | Même route, JSON `{"is_visible":false}` | N’importe quel statut 2xx est accepté. |

Sources : `client.go:238`, `260`, `777`. Il n’y a pas de requête HTTP DELETE. **Cela ne permet pas de conclure que la suppression est réversible** : la signification de `is_visible` appartient au backend distant. Le texte « may be hard to recover » minimise potentiellement le risque. Pas de sauvegarde complète, corbeille propre au projet, restauration ou vérification après mutation.

Les IDs sont concaténés au chemin sans validation/échappement. Après le 2xx, le client masque localement l’ID. Si cette écriture échoue, une opération distante possiblement réussie devient une erreur générique ; si le 2xx ne représente pas un succès métier, le cache peut au contraire simuler un succès.

La TUI impose une sélection puis un choix d’action et une confirmation. L’archivage est proposé par défaut ; aucune mutation n’est lancée au simple chargement. Toutefois Entrée confirme aussi la dernière étape : une suite d’Entrées peut franchir rapidement les écrans. Seulement huit titres sont montrés, sans revue complète des IDs ni confirmation de l’identité du compte. Les lots sont séquentiels, avec un délai global nominal de cinq minutes ; pas de temporisation dédiée, gestion `Retry-After`, transaction, rollback ou reprise fiable. Une erreur individuelle ne stoppe pas le reste du lot.

## Vulnérabilités potentielles et défauts prioritaires

Gravités qualitatives selon impact et préconditions ; elles ne sont pas des scores CVSS. « Constat statique » signifie que la branche de code est présente, pas qu’une exploitation a été tentée.

| ID | Gravité | Preuve / scénario | Conséquence et correction attendue |
| --- | --- | --- | --- |
| S01 | Élevée | `client.go:430,866,1042` journalise le corps JSON de session avant troncature. | En `--debug` ou `DEBUG=true`, des fragments d’access token et des informations personnelles peuvent apparaître. Tronquer n’est pas expurger. Supprimer les corps sensibles et les fragments de secrets des logs/rendus. |
| S02 | Élevée | `auth_state.go:15,78` et `client.go:575` persistent les secrets en clair. | Vol de session par un processus du même utilisateur, sauvegarde ou fichier trop permissif. Keychain macOS, minimisation, écritures atomiques protégées ; ne pas promettre une protection contre un compte utilisateur entièrement compromis. |
| S03 | Élevée conditionnelle | `client.go:351` demande `--remote-debugging-address=0.0.0.0`. Découverte CDP HTTP locale sans authentification propre. | Le contrôle CDP expose cookies et navigateur. L’écoute réellement obtenue dépend du navigateur et n’a pas été mesurée ; ne pas affirmer une exposition LAN confirmée. Imposer loopback, vérifier l’écoute et fermer le processus contrôlé. Course entre choix et ouverture du port. |
| S04 | Élevée | `model.go:285,335,405,501` conserve la sélection globale lors du filtrage ; « tout sélectionner » compare seulement des nombres. | Sélection A, filtre B, sélection B : A reste cible malgré son absence à l’écran. Le compte numérique apparaît mais peut être mal interprété. Définir une portée explicite, réviser le lot complet et tester les ensembles, pas leurs tailles. |
| S05 | Élevée | Cache global sans compte (`conversation_cache.go:25`), chargement cache/réseau parallèle (`model.go:144,153`). | Titres d’un compte précédent affichés ; intentions construites sur une identité non confirmée. Pas de preuve de mutation possible entre comptes côté serveur. Partitionner par identité/espace, lecture seule tant que la fraîcheur n’est pas établie. |
| S06 | Élevée | Toute réponse `loadResultMsg` force `phaseSelect` (`model.go:165`), même pendant confirmation/exécution. | Un refresh peut interrompre une confirmation ou rouvrir la sélection pendant un lot ; lot et affichage divergent, une nouvelle commande peut se mettre en attente. Invalider/fixer les générations et geler les cibles. Les mutex HTTP ne protègent pas cette machine à états. |
| S07 | Élevée | `model.go:927` utilise un contexte indépendant ; `q` quitte mais ne l’annule pas explicitement. | Des mutations peuvent continuer durant la sortie et leur état reste incertain si le processus s’arrête. Annulation explicite avant la cible suivante, progression et bilan des états inconnus. |
| S08 | Moyenne à élevée | `client.go:294,757,981` ferme le navigateur sans remettre `browserCtx` à nil. | Après bootstrap puis rejet d’auth, `ensureReady` peut réutiliser un contexte annulé au lieu de relancer Chrome. Défaillance déduite du flux, non reproduite avec un compte. Réinitialiser l’état et tester expiration/renouvellement simulés. |
| S09 | Moyenne | `evaluate(ctx,…)` ignore `ctx` et lance sur `c.browserCtx` (`client.go:870`). `startBrowser` crée des contextes Background. | Les délais annoncés et annulations ne couvrent pas réellement tout le travail ; blocage possible. Relier les opérations au contexte appelant et tester l’arrêt. |
| S10 | Moyenne à élevée | `doBackendRequest` lit sans limite, accepte tous les 2xx, suit les redirections par défaut ; cookies avec Domain `.chatgpt.com` et jar sans public suffix list. | Réponses incohérentes, consommation mémoire et portée des secrets trop large. Go retire certains headers aux redirections non apparentées, mais ce n’est pas une politique d’origine stricte : ne pas prétendre à une fuite arbitraire démontrée. Refuser redirections/origines inattendues, borner et valider. |
| S11 | Moyenne | `recordConversationVisibility` après mutation ; `SavedAt` du cache n’est mis à jour que si zéro. | Succès distant/échec disque indiscernables, marqueurs persistants et date de fraîcheur potentiellement ancienne. États distincts et réconciliation fiable. |
| S12 | Moyenne | `client.go:165,171,625` : lecteurs de session/email sans synchronisation commune avec leurs écritures. | Course possible lors du polling TUI concurrent avec l’auth ; `go vet` et les tests actuels ne l’exercent pas. Unifier les snapshots verrouillés et ajouter un test concurrent avec `-race`. |
| S13 | Moyenne | `MaskToken` retourne intégralement toute chaîne de longueur <=16 (`auth_state.go:141`), sinon révèle préfixe/suffixe. | Affichage inutile de fragments de secrets, indépendamment du debug. Retirer toute empreinte dérivée d’un jeton de l’interface. |
| S14 | Moyenne | `prepareAuthPath` du harness retire un chemin explicite existant avant tout test (`cmd/chatgpt-bulk-auth-e2e/main.go:236`). | Perte de fichier local si `--auth-file` désigne un vrai état, même sans stage de mutation distante. Refuser un chemin existant, isoler les fixtures et normaliser le chemin du binaire. Harness non lancé dans cet audit. |
| S15 | Moyenne | Titres/email/erreurs/corps distants rendus sans nettoyage explicite (`model.go:578,639,694,855`). | Risque de séquences de contrôle terminal, usurpation visuelle, lignes de confirmation trompeuses. Exploitabilité terminal exacte non testée ; filtrer contrôles ANSI/OSC, caractères de contrôle et textes multilignes. |
| S16 | Élevée pour la distribution | Bootstrap `bin` non vérifié et installation visant l’amont ; versions de livraison mouvantes. | Exécution d’un artefact non lié au fork audité ; renforcer provenance, versions, signatures et liens. Aucun installeur téléchargé/exécuté pour l’audit. |

Autres limites : le filtre saisit uniquement des chaînes d’un octet et efface un octet à la fois (`model.go:294,301`), donc recherche accentuée/emoji peu fiable ; les erreurs de rafraîchissement sont ignorées dès qu’un cache non vide existe ; les résultats d’un lot reviennent directement à la sélection si des conversations restent, sans écran de bilan systématique ; `Close` ne fait pas systématiquement un `browser.Close` sur tous les chemins d’erreur. Des renouvellements peuvent remplacer `profileDir` avant nettoyage de l’ancien profil. Ne pas garantir l’effacement immédiat ou sécurisé des profils.

## Compatibilité macOS Apple Silicon

**Compilation ARM64 : vérifiée ; exécution native macOS : non vérifiée.** `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build` réussit et produit un Mach-O ARM64. Le GoReleaser prévoit darwin/amd64 et darwin/arm64. Aucune dépendance CGO obligatoire dans le build testé ; Rosetta ne devrait pas être nécessaire pour le binaire ARM64, mais le navigateur choisi doit lui-même être compatible.

Le lancement des bundles `.app` utilise correctement la commande macOS `open` et un profil isolé. Restent à tester sur un vrai Mac : passage des arguments, découverte CDP, fin du processus détaché, nettoyages sur erreurs, permissions, terminal Unicode et comportement des navigateurs récents. Safari n’est pas pris en charge. Une `.app` graphique n’existe pas ; le RFC Wails est une proposition, pas une implémentation.

Pas de Developer ID/notarisation/DMG dans la configuration actuelle. La cible `make build` est native, mais les cibles d’archives font une conversion `x86_64 -> amd64` sans convertir `aarch64 -> arm64` (risque surtout Linux ARM). `make build-windows` ne fixe pas GOOS ; certaines cibles d’archives Windows supposent le bon hôte. La publication Ubuntu ne remplace pas une validation sur macOS.

## API internes et compatibilité probable avec ChatGPT actuel

Les trois contrats utilisés sont `/api/auth/session`, `/backend-api/conversations` et `/backend-api/conversation/{id}`. Ils sont traités ici comme **internes, sans contrat public stable démontré**. Ce ne sont pas les objets Conversations de l’API développeur OpenAI : remplacer le domaine par `api.openai.com` ne donne pas accès à l’historique ChatGPT d’un utilisateur.

| Élément | Appréciation au 8 novembre 2026 |
| --- | --- |
| Exécutable Go / format ARM64 | Fonctionne au build local, indépendamment du service distant. |
| Noms de cookies, fragmentation, champ `accessToken` | Hypothèses codées ; validité actuelle inconnue. |
| Cookie `__Host-…` réinjecté avec un attribut Domain | Incompatible avec les règles usuelles du préfixe `__Host-` ; comportement CDP/navigateur à vérifier, pas de succès supposé. |
| Login navigateur puis HTTP Go | Fragile si exigences de cookies supplémentaires, challenge, liaison au navigateur ou headers de compte évoluent. Le transfert ne copie pas tout le contexte navigateur. |
| User-Agent HTTP | Chaîne Windows Chrome/146 fixe, y compris sur Mac ; décalage avec le vrai navigateur. Ne justifie aucune tentative d’évasion des protections. |
| Réponse 403 | Toujours attribuée à une protection anti-bot, même si refus d’autorisation ou de compte ; diagnostic trop affirmatif. |
| Pagination / schéma / PATCH | Tests locaux uniquement. Ni disponibilité ni effet actuel confirmés. |

Le code ayant plus de sept mois à la date d’audit et reposant sur des détails privés, la probabilité d’une maintenance nécessaire est significative ; impossible de donner un taux fiable de fonctionnement. Une page publique accessible ou un 401/403 anonyme ne vérifierait pas ces contrats, et n’a donc pas été utilisé comme preuve.

Une vérification ultérieure autorisée devra être en lecture seule, après suppression des fuites de logs et mise en place d’une barrière technique bloquant les mutations. Observer une connexion normale et la liste sur une session contrôlée, sans exporter de secrets, puis comparer métadonnées et pagination à l’interface officielle. S’arrêter face aux contrôles de sécurité. Aucun test de suppression réelle n’est nécessaire ni autorisé dans le présent périmètre. Même une liste fonctionnelle ne validerait pas les PATCH.

## Validation locale exécutée

Sous Linux amd64, Go 1.24.2 déjà préparé, `GOTOOLCHAIN=local`, `GOFLAGS=-mod=readonly`, caches de modules existants et `GOPROXY=off` :

| Contrôle | Résultat |
| --- | --- |
| `go vet ./...` | Succès. |
| `go test -count=1 -json ./...` | 36 résultats de tests/sous-tests PASS, 0 FAIL, 0 SKIP ; quatre paquets testés, `internal/version` sans tests. |
| Build croisé `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /tmp/chatgpt-audit-darwin-arm64 ./cmd/chatgpt-bulk` | Succès. |
| `file /tmp/chatgpt-audit-darwin-arm64` | Mach-O 64-bit arm64 executable. Non exécuté. |
| `sh -n scripts/install-via-bin.sh` | Succès syntaxique uniquement ; script non exécuté. |
| `git diff --exit-code` avant ajout des livrables | Succès, aucun changement suivi préalable. |

Les tests de mutation existants vérifient le chemin HTTP et le masquage local contre un serveur factice ; ils ne vérifient même pas systématiquement le contenu PATCH. Pas de couverture des principaux risques de ce rapport : redaction, nouvelles sessions, interruptions réelles, ordonnancement des messages, chemins symlinks ou validation de contrat serveur. Pas de test macOS natif, de charge, de race detector ou de CVE exécuté pendant cet audit. Les succès de l’onboarding antérieur ne sont pas réinterprétés comme preuve d’une connexion ChatGPT.

## Décision proposée

**B. Réécrire certains composants.** Conserver Cobra, Bubble Tea, les modèles simples et le build Go ; remplacer par étapes la gestion de session, le transport/contrat et l’exécution sécurisée des lots. A serait moins coûteux immédiatement mais laisserait trop de logique sensible entremêlée. C ajouterait une régression et un coût de migration sans supprimer la dépendance aux interfaces internes. Une GUI macOS, Wails ou SwiftUI est un chantier distinct et ne rendrait pas ce backend plus fiable par elle-même.

Le détail de la proposition, les limites d’autorisation et les conditions de validation figurent dans [PLAN.md](PLAN.md). Aucune mise en œuvre n’a commencé ; validation utilisateur attendue.
