# Rapport de tests — 8 novembre 2026

Environnement de validation : Linux amd64, Go 1.24.2 ; données fictives et serveurs HTTP sur boucle locale. Aucune connexion à un compte ChatGPT, aucune suppression/archivage réel et aucune publication.

## Vérifications automatisées locales

- `go test -race -count=1 -tags ci ./...` : **84 tests et sous-tests réussis, 0 échec, 0 ignoré** ; six paquets testés. `internal/version` ne contient aucun test. Pilote Fyne virtuel et détecteur de courses activés.
- Parcours Fyne simulés : chargement et rechargement de trois exemples, recherche et filtre, sélection, protection, aperçu complet et exclusion des protégées, simulation de suppression et d’archivage, progression, erreur, annulation, fermeture de fenêtre, refus d’un second lot et redémarrage avec protections conservées.
- Moteur et stockage : absence de marqueur, import explicite, données incohérentes, liste modifiée sans empreinte correspondante, concurrence de 12 processus sur les protections, synchronisation avec une mutation simulée en cours, rejet des IDs inconnus ou protégés et annulation de l’attente d’un verrou.
- Inventaire et HTTP : pages de tailles différentes, total manquant ou variable, page prématurément vide, doublon, ID invalide, réponse trop volumineuse, erreur réseau et redirection. Les requêtes de mutation de tests sont adressées exclusivement à un faux serveur local.
- Identifiants : magasin système simulé, absence de nouveau fichier de secrets en clair, import explicite de l’ancien fichier, absence de secrets synthétiques dans les erreurs et logs vérifiés. Le test natif du Trousseau est fourni mais n’a pas été exécuté ici.
- `go vet -tags ci ./...`, vérification de syntaxe du script de bundle, vérification des modules `go mod verify` : exécutés localement.
- Compilation croisée Darwin ARM64 de la TUI et du binaire de test Trousseau : exécutables Mach-O ARM64 produits sous `/tmp`, non exécutés. Cela ne compile ni ne valide la GUI native Fyne.
- La GUI a aussi été démarrée avec le pilote virtuel `ci` : sortie normale ; ce pilote ne montre pas une fenêtre macOS. L’environnement a émis un avertissement Fyne sur la locale `C`.

## Risques et validations restantes

Les actions réelles sont désactivées dans l’interface **et** le client. Les contrôles de pagination ne prouvent pas que le service a fourni un instantané complet : son contrat privé n’a pas été vérifié et une modification du compte peut ne pas changer le total. L’identité/espace de travail et le résultat d’une requête déjà envoyée restent à valider ; aucune activation ne doit en être déduite.

La somme de contrôle locale détecte les changements incohérents, sans protéger contre un acteur qui contrôle simultanément les deux fichiers. Les verrous sont coopératifs et dépendent du système de fichiers. Le fonctionnement natif d’`fsync`, des verrous et du Trousseau sur le Mac de destination reste à tester. Un navigateur détaché peut subsister après certaines erreurs de lancement ; l’annulation a été vérifiée au niveau des contextes et des simulations, pas avec un navigateur macOS réel.

Le bundle graphique, son icône, son installation, le rendu et la signature/notarisation n’ont pas été validés sur macOS. Suivre la procédure et la liste de vérifications dans [DEVELOPPEMENT.md](DEVELOPPEMENT.md). Une validation de lecture ChatGPT requiert une autorisation distincte ; aucun test de mutation réelle n’est nécessaire à cette étape.
