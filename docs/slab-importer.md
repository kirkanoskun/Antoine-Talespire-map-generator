# Importer des bâtiments depuis une conversation

L'importeur accepte les **codes Slab v2** copiés depuis TaleSpire ou TalesTavern,
avec ou sans les triples accents graves. Il ne télécharge aucune page et n'accepte
pas les liens Board `talespire://`. Les autres versions sont refusées explicitement.

## Ce qu'Antoine peut envoyer

Coller un code par bâtiment dans la conversation. Un nom est utile ; auteur,
lien source et description sont facultatifs. Si seul le code est fourni, utiliser
un identifiant neutre (`slab_` suivi d'un fragment de son empreinte), sans inventer
le nom, la fonction ou l'auteur de la construction.

Pour l'assistant qui reçoit le code :

1. Récupérer le dépôt à jour et conserver le code exact dans un fichier temporaire.
2. Exécuter `go run ./cmd/import-slab -input <fichier> -id <id> -name <nom>`.
   Le catalogue portable `configs/prefabs.json` est mis à jour uniquement après
   validation. Enregistrer l'auteur et la source seulement s'ils sont connus.
3. Exécuter les tests puis proposer le changement dans une branche et une PR.
   Le test `TestEmbeddedPrefabCatalogue` revalide aussi tous les codes embarqués.
   Si Go est indisponible, ajouter l'entrée JSON au catalogue et faire valider par
   la CI avant d'annoncer l'import réussi. Une entrée invalide doit être retirée
   ou corrigée ; ne jamais présenter son ajout textuel comme un import validé.
4. Une nouvelle compilation de l'application embarque ce catalogue. L'application
   déjà installée sur le Mac n'est **pas** mise à jour à distance par la conversation.

Exemple de commande (les métadonnées ci-dessous sont illustratives) :

```sh
go run ./cmd/import-slab -input /tmp/maison.txt -id maison_pierre -name 'Maison en pierre'
```

Le fichier catalogue contient une liste d'objets avec `id`, `name`, `code`, et,
facultativement, `author`, `source_url`, `description`, `width`, `length`.
Le code est sauvegardé dans ce fichier ; les coordonnées et l'empreinte sont
recalculées à chaque chargement. Un import ne remplace jamais un ID existant.

## Import directement dans l'application

Ouvrir **Bâtiments importés** dans la première étape, saisir un identifiant et
coller le code. L'import est disponible immédiatement pour le prochain prompt,
sans appel à un modèle. Les imports locaux sont sauvegardés dans
`~/.talespire/prefabs.json`, distinct du catalogue embarqué. Le paramètre serveur
`-prefabs` permet de choisir un autre fichier local. Ne pas partager ce fichier
entre plusieurs processus écrivains ; le serveur sérialise ses propres imports.

API :

- `POST /api/prefabs/import` : un objet du format catalogue, retourne ses métadonnées.
- `GET /api/prefabs` : liste triée des métadonnées, sans les codes.

Les CLI `generate` et `describe` chargent le catalogue embarqué par défaut.
`-prefabs chemin.json` utilise un catalogue externe à la place.

## Placement dans une carte

L'IR possède un tableau optionnel `buildings` :

```json
{
  "prefab": "maison_pierre",
  "position": {"x": 8, "y": 7},
  "rotation": 90
}
```

Cet objet va dans `buildings`, au même niveau que `map`, `zones`, `connections`.
L'identifiant doit exister. La position indique le coin nord-ouest de l'emprise
réservée en tuiles. La rotation est un angle autour de l'axe vertical du jeu,
parmi 0, 90, 180 et 270 degrés. Les rotations de 90 et 270 échangent largeur et
longueur. Le modèle reçoit les IDs, dimensions et descriptions, jamais le base64.

Le moteur conserve les positions relatives au centième d'unité, les rotations
par objet, les identifiants d'assets et les bits supplémentaires. Il normalise
le point le plus bas au sol, aplanit l'emprise sur son altitude maximale, supprime
la dispersion de végétation dans cette zone et signale les POI qui l'occupent.
Il refuse les chevauchements entre emprises, les sorties de carte et l'eau.
L'aperçu montre une emprise dorée, pas les murs et les toits en 3D.

Le découpage garde chaque bâtiment entier dans la tranche de son point d'ancrage.
Il peut donc dépasser du rectangle nominal de cette tranche ; aligner les origines
des tranches selon la grille, sans les rapprocher à partir de leur bord visible.
Une tranche trop volumineuse est signalée et doit être réduite autrement.

## Limites et validation

- L'emprise automatique repose sur les **origines** des assets, avec marge, et
  non sur leurs maillages. Les débords de toit et gros objets ne peuvent pas être
  déduits avec certitude. `width` et `length` permettent d'agrandir la réservation.
- Les origines des arbres hors emprise peuvent avoir des branches qui y entrent.
  Il ne s'agit pas d'un moteur de collision de maillages 3D.
- Un Slab contenant déjà du terrain, un sous-sol ou des objets suspendus peut
  nécessiter un ajustement dans TaleSpire après import. La normalisation verticale
  utilise l'origine la plus basse, pas une porte ou un plancher identifié.
- Limites d'import : 1 Mio de texte, 30 Kio compressés, 4 Mio décompressés,
  100 000 assets. Par carte : 100 bâtiments, 50 000 assets importés.
- L'app conserve les IDs du code sans vérifier la disponibilité des assets dans
  l'installation TaleSpire cible. Aucun test en client TaleSpire n'est revendiqué.
- Les tests utilisent une fixture synthétique construite indépendamment de
  l'encodeur de l'app : précision, quatre rotations, aller-retour, limites, données
  invalides, sauvegarde, intégration HTTP, instructions et découpage.

## Référence du format

[Bouncy Rock, Slab Format V2](https://github.com/Bouncyrock/DumbSlabStats/blob/master/format.md),
consulté le 6 septembre 2026. Le codec importé est indépendant de `talescoder`
pour éviter les arrondis entiers de ses adaptateurs d'axes. La génération sans
bâtiment conserve le chemin d'export existant.
