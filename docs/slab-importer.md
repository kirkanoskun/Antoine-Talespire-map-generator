# Importer des bâtiments depuis une conversation

L'importeur accepte les **codes Slab v2** copiés depuis TaleSpire ou TalesTavern,
avec ou sans les triples accents graves. Il ne télécharge aucune page et n'accepte
pas les liens Board `talespire://`. Les autres versions sont refusées explicitement.

## Ce qu'Antoine peut envoyer

Coller un code par bâtiment dans la conversation, **avec sa licence** (affichée sur
la page TalesTavern ou TalesBazaar du bâtiment, ou l'accord explicite de son
créateur) et, si la licence l'exige, le nom de son créateur. Un nom de bâtiment
est utile ; lien source et description sont facultatifs. Si le nom manque,
utiliser un identifiant neutre (`slab_` suivi d'un fragment de son empreinte),
sans inventer le nom, la fonction ou l'auteur de la construction.

**Sans licence connue, un bâtiment ne peut pas entrer dans le dépôt** : le dépôt
est public et l'application est distribuée, donc ces bâtiments y sont redistribués.
Il reste possible de l'importer localement depuis l'application (voir plus bas).

Pour l'assistant qui reçoit le code :

1. Récupérer le dépôt à jour et conserver le code exact dans un fichier temporaire.
2. Exécuter `go run ./cmd/import-slab -input <fichier> -id <id> -name <nom>
   -license <licence> [-author <créateur>] [-source <lien>]`.
   `-license` est obligatoire, et `-author` aussi dès que la licence exige
   l'attribution (toute licence Creative Commons « BY »). Le catalogue portable
   `configs/prefabs.json` est mis à jour uniquement après validation. Ne jamais
   inventer une licence ou un auteur : sans eux, l'import est refusé, et c'est voulu.
   La commande affiche le niveau de sol détecté (voir « Niveau de sol ») : le
   vérifier.
3. Exécuter les tests puis proposer le changement dans une branche et une PR.
   Le test `TestEmbeddedPrefabCatalogue` revalide aussi tous les codes embarqués.
   Si Go est indisponible, ajouter l'entrée JSON au catalogue et faire valider par
   la CI avant d'annoncer l'import réussi. Une entrée invalide doit être retirée
   ou corrigée ; ne jamais présenter son ajout textuel comme un import validé.
4. Une nouvelle compilation de l'application embarque ce catalogue. L'application
   déjà installée sur le Mac n'est **pas** mise à jour à distance par la conversation.

Exemple de commande (les métadonnées ci-dessous sont illustratives) :

```sh
go run ./cmd/import-slab -input /tmp/maison.txt -id maison_pierre -name 'Maison en pierre' \
  -license 'CC BY-NC 4.0' -author 'Créatrice' -source 'https://talestavern.com/…'
```

Le fichier catalogue contient une liste d'objets avec `id`, `name`, `code`,
`license` (obligatoire dans le catalogue du dépôt : le test
`TestEmbeddedPrefabCatalogue` échoue sinon), et, facultativement, `author`
(obligatoire si la licence exige l'attribution), `source_url`, `description`,
`width`, `length`, `ground_y`.
Le code est sauvegardé dans ce fichier ; les coordonnées et l'empreinte sont
recalculées à chaque chargement. Un import ne remplace jamais un ID existant.

## Import directement dans l'application

Ouvrir **Bâtiments importés** dans la première étape, saisir un identifiant et
coller le code. La licence y est facultative : ces imports restent sur votre
machine et ne sont pas redistribués. Ce panneau n'existe que dans l'application
de bureau ; le site public n'accepte aucun import. L'import est disponible immédiatement pour le prochain prompt,
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
par objet, les identifiants d'assets et les bits supplémentaires. Il aplanit
l'emprise sur son altitude maximale, y pose le bâtiment par son **niveau de sol**
(voir ci-dessous), supprime la dispersion de végétation dans cette zone et signale
les POI qui l'occupent.

### Niveau de sol

Un bâtiment est posé par son **rez-de-chaussée**, pas par sa pièce la plus basse.
Beaucoup de constructions ont une cave ou des fondations destinées à être enterrées ;
les poser par leur point le plus bas les ferait flotter d'exactement cette
profondeur (c'est arrivé en jeu avec une auberge à 8,5 paliers de cave).

- Le niveau de sol est détecté automatiquement : c'est le niveau qui porte le plus
  de pièces, presque toujours le rez-de-chaussée. En cas d'égalité, le plus bas.
- Il est exprimé dans `ground_y`, en centièmes d'unité au-dessus de la pièce la plus
  basse (50 = un palier de terrain). Il apparaît dans les métadonnées et dans la
  liste de l'application. S'il est faux, réimporter avec `-ground <valeur>`
  (ou `ground_y` dans le catalogue).
- TaleSpire n'a pas d'altitude négative : si une cave descendrait sous zéro, toute
  la scène est remontée d'autant de paliers que nécessaire.
- Le terrain de la carte est creusé sous les pièces enterrées, pour que la cave ne
  soit pas remplie de terre. Le terrain généré n'a qu'un bloc d'épaisseur : vue de
  côté, une cave peut apparaître sous cette croûte.
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
- Un Slab contenant déjà du terrain ou des objets suspendus peut nécessiter un
  ajustement dans TaleSpire après import. Le niveau de sol est une heuristique
  (le niveau le plus peuplé), pas une porte ou un plancher identifié : le
  vérifier à l'import et le corriger avec `-ground` si besoin.
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
