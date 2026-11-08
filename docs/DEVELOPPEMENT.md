# ChatGPT Cleaner : développement et limites

État au 8 novembre 2026 : prototype Fyne, **simulation uniquement pour les actions**. La lecture du compte par le bouton dédié requiert une confirmation et n'a pas été testée. Le client Go refuse aussi l'archivage et la suppression réels. L'interface TUI historique ne peut donc plus modifier le service. Ne pas contourner cette protection pour utiliser l'application avec un compte réel.

## Interface

Le mode simulation charge trois titres fictifs. Il propose recherche, sélection multiple, protection, revue complète, simulation d’archivage ou de suppression, progression et annulation. Un filtre modifié vide la sélection. Les résultats simulés réussis sont retirés de la liste fictive ; le bouton Simulation recharge les exemples.

Les protections sont conservées par UUID dans `protected.json`. Sur Linux et macOS, un verrou de fichier sérialise les programmes coopérants qui utilisent ce dépôt ; les actions y conservent le verrou entre la vérification et l’envoi. L’écriture des protections et de leur empreinte utilise des fichiers temporaires, des renommages et `fsync`. Un fichier absent, mal formé ou incohérent avec son marqueur bloque l’opération. Après une interruption ou une corruption, ne recréez pas une liste vide : restaurez ensemble `protected.json` et `protected.json.initialized` depuis une sauvegarde cohérente, puis vérifiez la liste. Le fichier `.lock` sert à la synchronisation et doit rester en place pendant l’utilisation.

L’empreinte détecte une incohérence accidentelle : elle n’authentifie pas les fichiers contre une personne pouvant modifier à la fois la liste et son empreinte. Le verrou est coopératif ; il ne protège pas d’un programme qui ignore le verrou ou le remplace, ni d’un changement fait sur le service web. Si une sauvegarde efface simultanément le marqueur et la liste, le programme ne peut pas distinguer cette perte d’une toute première installation. Une liste existante sans marqueur est refusée : après avoir vérifié personnellement qu’il s’agit bien de votre ancienne liste, importez-la sans la modifier avec `go run ./cmd/chatgpt-bulk protections import-legacy`. Ne lancez pas cette commande pour masquer une corruption ou une perte de marqueur. Ces limites interdisent de qualifier la protection d’absolue.

## Authentification

Les nouveaux secrets persistants sont confiés au magasin système via `go-keyring` : Trousseau sur macOS, Secret Service sur Linux, gestionnaire d'identifiants sous Windows. Si le magasin est indisponible, l'opération renvoie une erreur, sans enregistrer de jeton en clair. Le compte du magasin est dérivé du chemin historique ; conserver la même valeur de `CHATGPT_BULK_AUTH_FILE` pour retrouver un profil.

Les anciens fichiers `auth.json` ne sont pas importés automatiquement ni supprimés. Pour importer explicitement un ancien profil :

```sh
go run ./cmd/chatgpt-bulk auth migrate-legacy
```

La commande refuse d'écraser un élément sécurisé existant et conserve l'ancien fichier : après contrôle personnel du profil importé et de vos sauvegardes, supprimez vous-même les anciennes copies si souhaité. La migration n'a été testée qu'avec des données fictives et un magasin simulé. Le Trousseau macOS réel reste à tester. `auth status` vérifie une présence, pas une connexion distante ; `logout` ne révoque pas la session côté ChatGPT et ne supprime pas l'ancien fichier. Le harness historique d'authentification fondé sur l'existence d'un fichier n'est pas adapté au nouveau stockage.

## Construire sur macOS Apple Silicon

Sur un Mac Apple Silicon, installer Go 1.24.2 ou une version compatible, Xcode Command Line Tools (`xcode-select --install`) et, pour une future connexion volontaire, un navigateur Chromium compatible. Vérifier l’outil Go et le SDK, puis construire dans le dépôt :

```sh
./scripts/package-macos.sh
open 'dist/ChatGPT Cleaner.app'
```

Le script vérifie que la destination n’existe pas, construit un binaire ARM64, crée un bundle `.app` avec icône et licence dans un dossier temporaire, puis le place dans `dist/`. Pour reconstruire, choisir une autre destination, par exemple `./scripts/package-macos.sh "dist/ChatGPT Cleaner test.app"`. L’outil `sips`, `iconutil` et le SDK Apple sont nécessaires. Le binaire peut être contrôlé avec `file 'dist/ChatGPT Cleaner.app/Contents/MacOS/chatgpt-cleaner'`. Il ne signe ni ne notarise l'application : une distribution fiable exige Developer ID, signature, notarisation et vérification séparées. La compilation et l'ouverture sur macOS n'ont pas été exécutées dans cet environnement Linux. Le script n'a été vérifié que syntaxiquement.

Fyne a été choisi pour ses widgets Go et son modèle de distribution natif. Fyne requiert CGO, un compilateur C et les bibliothèques graphiques du système ; un build macOS nécessite le SDK Apple. Wails conserve également Go, mais nécessite une interface web et WebKit, en plus des outils natifs ; aucune des deux solutions ne permet de valider un bundle macOS sans cette plateforme. Une substitution Go épingle Fyne au tag officiel GitHub v2.6.3 pour les environnements où le domaine `fyne.io` est inaccessible.

Sur Linux, `go test -tags ci ./...` utilise le pilote Fyne sans fenêtre. Sans le tag, installer les dépendances X11/OpenGL de Fyne pour construire l'interface native. La compilation croisée `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/chatgpt-bulk` ne valide que la TUI, pas la GUI.

## Limitations du service

L'application utilise des routes privées de ChatGPT et une authentification issue du navigateur, sans contrat public stable confirmé. Aucune requête ChatGPT authentifiée ni opération réelle n'a été réalisée pour ces tests. La pagination accepte uniquement des pages avec `items` et `total`, un total constant, des UUID distincts et exactement le nombre annoncé ; sinon l’inventaire est invalidé. Une réponse volumineuse, une redirection HTTP ou une réponse incompatible provoque également une erreur. Ces contrôles ne constituent pas une preuve d’exhaustivité : les routes sont internes et un historique peut changer pendant le chargement sans changer de total. La liste des espaces de travail, la sémantique et la disponibilité réelles restent inconnues. Une erreur pendant une mutation simulée ou future interrompt le lot ; une requête déjà envoyée pourrait avoir un résultat inconnu et ne doit pas être répétée automatiquement. Ne pas contourner MFA, challenges ou contrôles d'accès.

## Vérifications obligatoires sur Mac avant toute livraison

- Compiler le bundle sur Apple Silicon avec le SDK Apple ; vérifier architecture, lancement, thème, taille, accessibilité, clavier et fermeture.
- Vérifier le stockage, récupération, refus d'accès, migration explicite et effacement d'un secret **fictif** dans le Trousseau ; vérifier qu'aucune valeur n'apparaît dans les arguments ou logs.
- Tester visuellement le chargement fictif, la recherche, le filtre, la sélection multiple, la protection, l’aperçu complet, le mode « Tout sauf protégées », la progression, l’erreur, l’annulation et le redémarrage. Lancer deux instances et vérifier qu’aucune protection n’est perdue ; ne pas essayer de mutation réelle.
- Avec autorisation distincte, valider la connexion normale et la lecture seule, sans communiquer de secret. Ne pas exécuter de mutation réelle durant la validation.
- Vérifier les verrous et `fsync` sur le système de fichiers du Mac ; ne pas extrapoler à un lecteur réseau. Examiner la gestion d’identité/espace et les changements du serveur pendant la pagination, le nettoyage du navigateur après interruption et la sémantique d’une réponse perdue. Réaliser un audit avant toute décision d’activer les mutations.
- Dans le dossier du dépôt, exécuter `go test -tags ci -race ./...` pour les tests Fyne sans fenêtre, puis `go test -tags keychain_integration ./internal/chatgpt -run TestNativeKeychainRoundTrip -count=1` : ce test optionnel crée puis retire uniquement une entrée de Trousseau fictive associée à un profil temporaire. Vérifier également le refus d’accès au Trousseau et la migration d’un faux ancien fichier.
- Pour distribuer : disposer d’une identité Developer ID, signer le bundle et le notariser selon la procédure Apple, puis contrôler `codesign --verify --deep --strict` et Gatekeeper sur une machine indépendante. Le script fourni ne signe ni ne notarise ; il ne demande aucun secret et ne publie rien.
