# Spec Lot 1 : catalogue complet TaleSpire et thèmes de scène

Projet : Le Cartographe (`kirkanoskun/Antoine-Talespire-map-generator`)
Version : v2, 7 novembre 2026 (v1 du 27 octobre 2026, amendée après revue du code et des deux index).
Destinataire : session Claude Code sur le dépôt.
Statut : implémenté (7 novembre 2026) ; reste la validation en jeu par Antoine (US5), avec `testdata/scene_place_village.json` et `testdata/scene_cimetiere_marais.json`.
Mode : spec déléguable. Lire `CLAUDE.md` avant de commencer, ses règles priment (un biome par carte, IR validée à la frontière, sorties déterministes).

## Ce qui a changé depuis la v1

Chaque point ci-dessous a été vérifié contre le code et contre les deux `index.json` versionnés dans `configs/taleweaver/`.

| # | Changement | Raison |
|---|---|---|
| A1 | Non-régression : **59** identifiants uniques (props.json + biomes.json), et non 67 | `props.json` a 71 parties pour 59 identifiants distincts ; `biomes.json` n'en ajoute aucun. Les 59 sont présents et non obsolètes dans l'index. |
| A2 | Les objets du catalogue passent par le **chemin d'encodage des bâtiments** (`slab.Layout`), pas par talescoder (RG4) | talescoder ne place qu'à la case entière et décale un axe ; le chemin des bâtiments est exact au centième. C'est aussi ce qui garde le golden intact. |
| A3 | Rotation des objets du catalogue **fixée à 0°** en lot 1 | Décision d'Antoine. Les POI de l'ancien catalogue gardent leur rotation aléatoire (golden). |
| A4 | Un objet du catalogue **réserve son emprise** (RG7) | Aujourd'hui la végétation est posée avant les POI : un arbre peut pousser dans la fontaine. |
| A5 | Relief pavé : bloc **`castle floor 1x1`** au lieu de « CobbleStone Floor Small » ; clé de relief `paving` | Les tuiles CobbleStone font 0,25 case de haut, les blocs de colonne 0,5 (une marche) : empilées, elles laissent des jours. |
| A6 | Relief `swamp_floor` : son bloc existe déjà dans `props.json` (`mud_small` = « Swamp floor 1x1 ») ; les végétaux de marais entrent dans `props.json` | Les props d'un relief ne se résolvent que via `props.json` (`TestConfigReferentialIntegrity`). |
| A7 | Les nénuphars vont dans le relief `water` du marais (seule modification d'un relief existant, autorisée car le golden est en `temperate_forest`) | Un nénuphar sur la terre ferme n'a pas de sens. |
| A8 | Un `prop` inconnu déclenche la correction (avec clé) et un message à renvoyer à l'IA (sans clé) | Le modèle ne voit que les thèmes : il inventera des slugs, qui seraient ignorés en silence. |
| A9 | RG2 complété : nom vide, doublons mesurés (99 slugs, 263 entrées) | Une entrée de l'index a un nom vide ; 84 doublons persistent après le suffixe de dossier. |
| A10 | `assets.json` : environ 1 Mio (mesuré avec les tags), empreinte `footprint` calculée à l'import (RG5) | Le générateur n'a pas à recalculer l'emprise ; le calcul est testé une fois. |
| A11 | `nl.PromptVersion` passe à 1.05 ; questions ouvertes tranchées (section 11) | |

---

## 1. Contexte et problème

Le générateur ne connaît que 59 assets distincts, hérités de taleslab (`configs/props.json`). Le jeu en contient 2 875 non obsolètes dans ses deux packs officiels. Résultat : impossible de produire une scène précise ("un cimetière au milieu d'un marécage", "une place avec une fontaine au milieu", "un village abandonné"). Le modèle n'a ni tombe, ni fontaine, ni étal, et l'alias `altar -> stone_big` illustre le bricolage actuel. Autre exemple découvert en revue : le sol du biome `swamp` (`dead_ground_small`) est en réalité « Lava floor 1x1 ashes ».

Le catalogue complet existe : TaleSpire embarque un `index.json` par pack, au format documenté par Bouncyrock (TaleWeaver Community AssetPack Index Format). Antoine les a extraits de son installation Mac :

`~/Library/Application Support/Steam/steamapps/common/TaleSpire/TaleSpire.app/Contents/Resources/Taleweaver/<pack-uuid>/index.json`

| Pack | UUID du pack | Tuiles | Objets | Non obsolètes |
|---|---|---|---|---|
| Medieval Fantasy | `d71427a1-5535-4fa7-82d7-4ca1e75edbfd` | 1 396 | 1 035 | 2 025 (1 009 tuiles, 1 016 objets) |
| Cyberpunk and Sci-fi | `fc3a4efc-e8d9-4d82-8620-f26c5b66023c` | 364 | 494 | 850 (360 tuiles, 490 objets) |

Les deux fichiers sont versionnés : `configs/taleweaver/medieval-fantasy.index.json` et `configs/taleweaver/scifi.index.json`.

**Décision d'Antoine** : les deux index sont versionnés dans le dépôt et le catalogue complet est embarqué partout (app desktop et site), Sci-fi compris. Il assume ce choix et retirera les fichiers si Bouncyrock le demande.

## 2. Utilisateurs et job à faire

MJ TaleSpire francophone qui prépare une séance. Job : "décrire une scène précise et obtenir une carte où les éléments demandés sont réellement présents, au bon endroit, avec les vrais objets du jeu".

## 3. Objectif et mesure de succès

- **Objectif** : le modèle peut placer n'importe quel objet ou tuile du jeu, et dispose de sélections thématiques pour les scènes courantes.
- **Mesure** : sur les 4 scènes de référence (US5), chaque élément nommé dans la description (fontaine, tombes, clôture, roseaux, étals, murs en ruine) apparaît dans la carte avec un objet du thème, pas un substitut générique, et aucun objet ne chevauche un autre ni un arbre. Contrôle visuel dans l'aperçu 2D, puis en jeu pour au moins une scène.

## 4. Périmètre

**Dans le lot 1**
- Importeur `index.json` vers un catalogue compact versionné.
- Résolution des points d'intérêt (POI) sur tout le catalogue, encodage exact, rotation 0°, emprise réservée.
- 4 thèmes de scène curés, injectés dans les prompts (avec clé et sans clé).
- 2 nouveaux reliefs de sol : pavé (`paving`) et sol de marais (`swamp_floor`) ; nénuphars dans l'eau du marais.
- Aperçu 2D : emprise réelle des POI issus du catalogue.
- Retour à l'IA quand elle nomme un objet inconnu.

**Hors périmètre (et pourquoi)**
- **Motifs de placement** (rangées de tombes, objets en cercle, clôture en bordure) : lot 2. Le lot 1 se contente des positions `{x,y}` et du `scattered` existants.
- **Rotation choisie des POI** : lot 2 (clôtures en bordure, étals tournés vers la place). En lot 1, 0° pour tout objet du catalogue.
- **Assemblage de bâtiments mur par mur** (kits "Abandoned Village" et "Rural") : lot 3. En lot 1, les murs en ruine servent de POI isolés.
- **Objets composites à empiler** (arbre de marais base, milieu, sommet) : lot 2, les décalages verticaux sont à valider en jeu.
- **Tuiles de sol 2x2 et tuiles de 0,25 case** (CobbleStone) : le générateur pose une tuile d'une marche par case et par niveau.
- **Créatures et musiques** de l'index : sans usage aujourd'hui, non importées.
- **Thèmes Sci-fi** : le pack est catalogué mais aucun thème n'est écrit en lot 1.
- **Corriger le sol actuel du marais** (`dead_ground_small` = cendres de lave) : hors lot, le golden et les cartes existantes en dépendent ; le relief `swamp_floor` offre le bon sol.

## 5. Parcours principal

1. Le MJ décrit "une place de village pavée avec une fontaine au centre et des étals autour".
2. Le prompt (desktop ou site) contient le catalogue de base actuel et la liste des 4 thèmes avec leurs objets.
3. Le modèle produit une IR : zone avec `relief_override: "paving"`, POI `fountain_01` au centre, POI `stall_fruit_01`, `stall_vegetables_01`, etc. à des positions explicites.
4. Le générateur résout chaque `prop` (RG3). Pour un objet du catalogue, il réserve et aplanit son emprise avant de semer la végétation, puis l'émet comme un `slab.Layout` exact, à 0°, posé sur le sol (RG4, RG7).
5. L'aperçu montre la fontaine sur 2x2 cases et les étals ; le MJ copie le code dans TaleSpire.

## 6. User stories et critères d'acceptation

**US1. Importer le catalogue**
En tant que mainteneur, je veux convertir les `index.json` en catalogue compact, afin de mettre à jour les objets quand le jeu évolue.
- [ ] `go run ./cmd/import-index -input configs/taleweaver/medieval-fantasy.index.json -input configs/taleweaver/scifi.index.json -out configs/assets.json` produit le catalogue.
- [ ] Le catalogue contient exactement les entrées `Tiles` et `Props` non obsolètes : 2 875 entrées pour les deux fichiers actuels.
- [ ] Deux exécutions successives produisent un fichier identique à l'octet (tri stable, pas d'itération de map Go).
- [ ] Un test vérifie que `configs/assets.json` est à jour par rapport aux index (même principe que `TestSampleMapIsCurrent`, avec `-update`).
- [ ] Chaque `slug` est unique et non vide (RG2).
- [ ] Non-régression : **chaque identifiant d'asset de `configs/props.json` et `configs/biomes.json` existe dans le catalogue** (59 aujourd'hui, tous présents et non obsolètes). Exemple : `fire` (`AAA0wD/Pk8fsSpNFmDCNyR1L`) correspond à "Bonfire", dossier Lights, UUID `cf3fc034-c793-4aec-9345-98308dc91d4b`.

**US2. Placer n'importe quel objet du jeu**
En tant que modèle (ou MJ éditant l'IR), je veux nommer un objet du catalogue dans un POI, afin de placer l'objet exact.
- [ ] Un POI `{"prop": "fountain_01", "position": {"x": 20, "y": 20}}` produit une slab contenant l'UUID `08545429-752c-41c5-9758-1279d82b70eb`, une seule fois, rotation 0.
- [ ] L'ordre de résolution de RG3 est respecté et testé.
- [ ] La position et la hauteur suivent RG4 (test : décoder la slab avec `internal/slab` et vérifier X, Y, Z de la fontaine et d'une tuile d'étal).
- [ ] L'emprise est réservée selon RG7 (test : aucune végétation ni rocher sous la fontaine ; un second POI qui la chevauche est refusé avec un avertissement).
- [ ] Ajouter un objet du catalogue ne change pas le reste de la carte : le code des cases hors emprise est identique avec et sans la fontaine (même graine).
- [ ] Le test golden (`internal/generator/golden_test.go`, `testdata/golden_scene*`) reste inchangé : une IR qui n'utilise que l'ancien catalogue produit exactement le même code.
- [ ] Un `prop` inconnu reste un avertissement pour le générateur (comportement actuel). En plus :
  - avec clé (`internal/nl`), l'IR est renvoyée au modèle avec l'erreur, comme une IR invalide, dans la limite des tentatives existantes ;
  - sans clé (`/api/generate`), l'avertissement est affiché en français, avec un message de correction prêt à copier pour l'IA (même mécanique que les erreurs `invalid_map`), et la carte est quand même dessinée.

**US3. Proposer des thèmes au modèle**
En tant que MJ, je veux que le modèle connaisse les objets pertinents pour ma scène sans lire 2 875 entrées, afin d'obtenir une IR juste et un prompt de taille raisonnable.
- [ ] `configs/themes.json` contient les 4 thèmes de la section 8, chaque objet référencé par UUID (pas par nom, à cause des doublons).
- [ ] Les thèmes sont ajoutés au catalogue du prompt (`nl.Catalog`, donc `PromptHead`) : identifiant, libellé français, et pour chaque objet son `slug`, son emprise en cases et un rôle court. Le prompt avec clé et le prompt sans clé du site en héritent ensemble.
- [ ] Le catalogue complet n'est **pas** injecté dans les prompts.
- [ ] Un test vérifie que chaque UUID de `themes.json` existe dans `assets.json`.
- [ ] `nl.PromptVersion` passe à `1.05` ; `TestPromptIsHeadPlusSceneLines` reste vert.

**US4. Sols pavés et sols de marais**
En tant que MJ, je veux qu'une zone soit pavée ou marécageuse, afin que le sol corresponde à la scène.
- [ ] Nouveau relief `paving` sur `temperate_forest` (et tout autre biome où une place de village a du sens, à l'appréciation du dev), bloc de colonne **"castle floor 1x1"** (UUID `a2eaf2e2-01d8-4548-a517-50df8551057a`, 1x1, une marche de haut, empilable comme les blocs de sol actuels). Densité de végétation nulle ou très faible.
- [ ] Nouveau relief `swamp_floor` sur `swamp`, bloc **`mud_small`** (déjà dans `props.json` : c'est "Swamp floor 1x1", UUID `503f0799-b652-4d81-b202-d4473caecee4`), végétation issue du thème marais (roseaux, prêles).
- [ ] Les végétaux de marais utilisés par un relief sont ajoutés à `props.json` (format actuel : `asset_parts` avec `dimensions` 1x1x1 et `offset_z` 1, comme `fire`), générés depuis le catalogue pour ne pas recopier d'identifiants à la main.
- [ ] Les nénuphars (`swamp lily pad 01/02`, `swamp lily flower`) remplacent le corail et le poisson dans la catégorie végétation du relief `water` du biome `swamp`. C'est la seule modification d'un relief existant ; le golden (biome `temperate_forest`) n'en dépend pas. Hauteur sur l'eau à valider en jeu.
- [ ] Les autres reliefs ne changent pas (golden inchangé).
- [ ] `config_sanity_test` couvre les nouveaux reliefs.

**US5. Vérifier sur les scènes de référence**
- [ ] Les 4 descriptions suivantes, ajoutées à `testdata/descriptions.txt`, produisent une IR valide via le prompt sans clé (Antoine colle la réponse de son assistant) :
  1. "un cimetière au milieu d'un marécage"
  2. "une place de village pavée avec une fontaine au milieu et des étals de marché"
  3. "un village abandonné, des maisons en ruine et une charrette renversée"
  4. "une clairière avec un feu de camp" (contrôle : le comportement actuel ne régresse pas)
- [ ] Dans l'aperçu 2D, les objets nommés sont visibles avec leur emprise réelle (la fontaine occupe 2x2 cases).
- [ ] **Validation manuelle en jeu** par Antoine sur le Mac : coller le code de la scène 2 dans TaleSpire, vérifier que la fontaine et les étals apparaissent à leur place, posés au sol (ni enterrés, ni flottants), et que les tombes de la scène 1 sont à demi enterrées comme prévu par le jeu. C'est aussi la première validation de la case "In-game import" du README.

## 7. Règles de gestion

**RG1. Format des identifiants (ferme, vérifié)**
`id` du catalogue = base64 standard de 18 octets : 2 octets nuls, puis l'UUID de l'index en ordre d'octets .NET (`Guid.ToByteArray`, équivalent Python `uuid.UUID(s).bytes_le`). C'est le format déjà utilisé par `configs/props.json`. `slab.Layout.ID` attend les 16 octets de l'UUID dans ce même ordre.

**RG2. Slug**
- Nom en minuscules, caractères non alphanumériques remplacés par `_`, `_` multiples fusionnés, `_` en tête et en fin retirés.
- Nom vide (une entrée de l'index) : `asset_` suivi des 8 premiers caractères de l'UUID.
- Doublon (99 slugs partagés par 263 entrées) : suffixe `_<dossier en slug>` ; si le doublon persiste (84 cas, variantes d'un même dossier), suffixe supplémentaire `_<4 premiers caractères de l'UUID>`. Vérifié : aucun doublon après ces deux étapes.
- Les slugs des objets des thèmes de la section 8 sont uniques sans suffixe (vérifié).

**RG3. Résolution d'un `prop` de POI**, dans cet ordre :
1. identifiant de `configs/props.json` (inchangé, chemin talescoder, rotation aléatoire actuelle) ;
2. alias de `internal/generator/aliases.go` (inchangé) ;
3. `slug` du catalogue (chemin exact, RG4) ;
4. sinon avertissement (et retour à l'IA, US2).

Deux slugs du catalogue coïncident avec des alias (`bonfire`, `campfire`) : l'alias gagne, et il désigne déjà le même objet. Les alias existants peuvent ensuite être repointés vers de meilleurs objets (ex. `altar` vers "Altar (Evil)"), mais seulement si le golden reste inchangé ; sinon, laisser pour plus tard.

**RG4. Encodage, position et assise des objets du catalogue**
Un objet du catalogue est traité comme un **bâtiment d'une seule pièce** : il réutilise la réservation, l'aplanissement, l'assise et l'émission `slab.Layout` de `placeBuildings` (`internal/generator/buildings.go`), avec des avertissements au lieu d'erreurs. C'est le choix le plus simple à maintenir : un seul code d'emprise et d'assise pour tout ce qui est posé précisément, et le chemin talescoder des 59 assets actuels reste intact (golden).
- **Emprise** (`footprint`, en cases) : `ceil(taille - 0,15)` sur X et Z, minimum 1 (fontaine 2,02 x 2,03 → 2x2 ; Stall Magic 2,01 x 1,12 → 2x1 ; wagon_01 2,00 x 4,54 → 2x5). La tolérance absorbe les débords de quelques centimètres des maillages.
- **Ancrage** : la case `{x,y}` du POI est le coin nord-ouest de l'emprise, comme pour `buildings`.
- **Position horizontale** : placer l'origine de l'objet pour que le centre de ses bornes tombe au centre de l'emprise : `X = (x + w/2 - centre.x) × 100`, `Z = (y + d/2 - centre.z) × 100`. Une seule formule pour tout : tuiles à origine en coin, objets à origine au centre, et les exceptions mesurées (127 objets dont "Graveyard fence corner" et 2 tuiles ont des bornes décentrées).
- **Hauteur** : l'origine est posée sur la surface du sol aplani, `Y = surface(h) = (h + 1) × 50`. Ne pas "corriger" avec les bornes : certaines descendent sous l'origine par conception ("Gravestone 01" : centre vertical 0,01, demi-hauteur 0,46, à moitié enterrée).
- **Rotation** : 0 (lot 1).
- **Unités** : dans l'index, 1 = une case ; dans la slab, 100 unités par case horizontalement, 50 par marche verticalement (une marche = une demi-case, rappel `CLAUDE.md`).
- À confirmer en jeu (US5) : que le jeu place bien l'origine de l'index à la position stockée. Si ce n'est pas le cas, seule la formule de position change.

**RG5. Champs du catalogue compact**
Par entrée : `id` (RG1), `uuid`, `slug`, `name`, `kind` (`tile` ou `prop`), `pack`, `folder`, `group` (`GroupTag`), `tags`, `size` `[x, z, y]` en cases (2 × `m_Extent`), `center` `[x, z, y]` (`m_Center`), `footprint` `[w, d]` en cases (RG4). Valeurs arrondies au millième pour un fichier stable. Ne pas importer `Assets` ni `Icon` : sous-maillages et icônes sont gérés par le jeu ; un objet en plusieurs parties (ex. "Stacked Crates", 3 parties) reste une seule entrée et un seul UUID dans la slab.

**RG6. Fichiers**
- Index bruts versionnés : `configs/taleweaver/medieval-fantasy.index.json` (6,8 Mo) et `configs/taleweaver/scifi.index.json` (2,1 Mo). Ils ne sont pas embarqués.
- Catalogue généré et versionné : `configs/assets.json`, environ 1 Mio (mesuré avec les tags). Seul ce fichier est embarqué (`embed.go`), lu depuis des octets par `internal/catalogs`.
- Thèmes : `configs/themes.json`, embarqué aussi.

**RG7. Réservation de l'emprise**
- Les objets du catalogue sont réservés dans la même passe que les bâtiments, **avant** la végétation : aucune végétation, aucun rocher, aucun autre POI sur leurs cases.
- L'emprise est aplanie à la hauteur la plus haute de ses cases (comme un bâtiment), pour que rien ne flotte ni ne s'enterre sur une pente.
- Chevauchement avec un bâtiment ou un autre objet du catalogue, ou débord hors carte : l'objet est ignoré avec un avertissement (pas d'erreur, contrairement aux bâtiments).
- `scattered` : chaque exemplaire réserve son emprise ; un tirage qui chevauche est retenté sur une autre case de la zone, puis abandonné avec un avertissement.
- La séquence aléatoire du reste de la carte ne doit pas changer : comme pour `HeightField.dug`, on tire les nombres comme avant et on n'omet la pose qu'au moment d'émettre.
- Pas de contrôle de l'eau en lot 1 (un POI peut être posé au bord d'une mare, comme aujourd'hui).

## 8. Thèmes de scène (contenu de `themes.json`)

Rôles indicatifs, à affiner. UUID et tailles vérifiés dans l'index Medieval Fantasy (tous non obsolètes, slugs uniques sans suffixe). Emprise selon RG4.

**`cimetiere` (Cimetière)**
| Objet | Type | Emprise | Rôle | UUID |
|---|---|---|---|---|
| Gravestone 01 | prop | 1x1 | tombe | 89e6faca-3b0c-41e6-bb2f-80c5c6eb0b03 |
| Gravestone 02 | prop | 1x1 | tombe | 0ec92df2-c6c9-4be4-85f9-ca340e102201 |
| Gravestone 03 | prop | 1x1 | tombe | 16865988-9e66-4add-a9ba-b5fc257afd5d |
| Gravestone 04 | prop | 1x1 | tombe haute | 7ae5095a-d2a8-4f87-ac91-936425e85326 |
| Gravestone With Crow 01 | prop | 1x1 | tombe, détail | 311c385d-6c34-4bcb-aa5a-dcf77bbb99eb |
| stoneMoniment01 | prop | 1x1 | monument | 33a69f83-2ced-42cd-a2a2-1aabe366f7db |
| stoneMoniment02 | prop | 1x1 | monument | c687de23-a17d-4362-8faf-2eefe6fcd7da |
| stoneMoniment03 | prop | 1x1 | monument | bc51680d-e791-462b-8d87-a96281dc9310 |
| Sarcophagus | prop | 1x2 | caveau, centre | 7d4b36c7-4c2a-4e10-9ce5-cca0b4c85cbe |
| Basic Coffin | prop | 1x2 | cercueil | 2827c7a5-8ae7-426f-a495-29ee382243d1 |
| statue_lg_hooded_01 | prop | 2x2 | statue centrale | 0a5aad94-8977-4e42-960c-a72159b10e6c |
| Graveyard fence 01 | prop | 2x1 | clôture 2 cases | 08b1918a-7cdb-4643-8d14-008cd6d18feb |
| Graveyard fence 02 | prop | 1x1 | clôture 1 case | 7a33b43e-ef84-4e06-90bf-333412d88cdb |
| Graveyard fence corner | prop | 2x2 | coin de clôture (bornes décentrées) | a94d0fed-79c3-4317-af28-4ab04fd7e04e |
| Graveyard fence broken 01 | prop | 1x1 | clôture cassée | 1e86ec31-8bfe-4db8-a489-2670468b980a |
| Graveyard fence beam | prop | 1x1 | poteau | 84ee0f89-1c06-467d-a3ef-7db0ef42768d |
| Doors - Graveyard fence | tile | 2x1 | portail | 48583acf-ab32-40cf-990e-68096e080dd7 |

Sans rotation (lot 1), les clôtures sont toutes orientées est-ouest : le prompt doit le dire au modèle (rôle "clôture, orientée est-ouest").

**`place_village` (Place de village)**
| Objet | Type | Emprise | Rôle | UUID |
|---|---|---|---|---|
| Fountain_01 | prop | 2x2 | fontaine, centre | 08545429-752c-41c5-9758-1279d82b70eb |
| Well 01 | prop | 1x2 | puits | ec5ceb75-0f02-4892-96bc-f82e55890a82 |
| Knight Statue | prop | 1x1 | statue | 54f1eaf1-63f2-4665-80f5-3287bd7cf79d |
| Statue 003 | prop | 1x1 | statue | bab76375-3286-4eec-a9b1-b41558166773 |
| castle floor 1x1 | tile | 1x1 | sol pavé (relief `paving`, pas un POI) | a2eaf2e2-01d8-4548-a517-50df8551057a |
| Stall Fruit 01 | tile | 1x1 | étal | 554ac0fb-4489-41dd-80ce-aec63f0a8649 |
| Stall Vegetables 01 | tile | 1x1 | étal | 829aef7b-412e-44d2-82b8-f9e425ad3c1f |
| Stall Vegetables 02 | tile | 1x1 | étal | adf23589-a725-476b-bde0-df7d1dab48ba |
| Stall Corner Spices | tile | 1x1 | étal d'angle | 579700f7-e809-43e0-b058-9e1aa5d37cd0 |
| Stall Empty 01 | tile | 1x1 | étal vide | ead12d1a-9acc-4fc6-81aa-9aef517f03b2 |
| Stall Magic | prop | 2x1 | étal | c2d077af-f8ff-4dca-a897-5ea243e04d5b |
| Stall Weapons | prop | 2x1 | étal | b357d530-becf-4a05-8a4d-feffa4b907d0 |
| Wooden Cart | prop | 1x1 | charrette | 8d13bc1e-f05b-4c03-8d57-5a897e66693e |
| wagon_01 | prop | 2x5 | chariot | 99d33645-b866-4b17-a093-07b2eef7f684 |
| Crate - Large | prop | 1x1 | caisse | d9dcd448-2c03-4b16-a67e-54d8f4b11a04 |
| Stacked Crates | prop | 1x1 | caisses | 2ed62f17-ca95-4705-a69d-6ee25930261a |

**`marais` (Marais)**
| Objet | Type | Emprise | Rôle | UUID |
|---|---|---|---|---|
| Swamp floor 1x1 | tile | 1x1 | sol (relief `swamp_floor`, = `mud_small`, pas un POI) | 503f0799-b652-4d81-b202-d4473caecee4 |
| swamp reed 01 | prop | 2x1 | roseaux | 63650e1d-6b11-4807-afd1-17aaf3330a68 |
| swamp reed 02 | prop | 1x1 | roseaux | a2d4ea14-e21c-43d2-a884-0037f6ac4c25 |
| swamp reed horsetail 01 | prop | 1x1 | prêles | 3dae85f6-7870-4751-8e14-9a07e15cdb4b |
| swamp reed horsetail 04 | prop | 1x1 | prêles | f162d284-f144-4c62-8346-050ba33e4161 |
| swamp lily pad 01 | prop | 1x1 | nénuphar (relief `water`) | 277edf44-e481-45a6-8a67-f91579e22778 |
| swamp lily pad 02 | prop | 1x1 | nénuphar (relief `water`) | 90e6eb4a-36ce-47db-b9dc-8ad6d69b12af |
| swamp lily flower | prop | 1x1 | fleur de nénuphar (relief `water`) | 2fa04957-41fa-480b-9099-8939dd25956d |
| Swamp tree Welcome arch | prop | 1x2 | arche de racines | 727132ed-63b0-484b-8184-e8e74d49cea8 |

**`village_abandonne` (Village abandonné)**
Tous trouvés dans l'index (dossier "Abandoned Village", `group` `wall`, `floor`, `stairs` ; chariots et puits dans "Misc. Exterior") : `wall_1x1_breach_01_left`, `wall_1x1_breach_01_right`, `haunted_wall_1x1_broken`, `abandoned_village_wall_1x1_corner_01`, `abandoned_village_wall_2x1_01`, `abandoned_village_floor_2x2_01`, `abandoned_village_stairs_broken_01`, `wagon_ruined_01`, `wagon_ruined_02`, `well_01` (slug de "Well 01"). Le dev complète avec quelques débris des dossiers "Grim" ou "Nature" à son jugement et remplit `themes.json` par UUID. Rôle : ruines posées comme POI isolés en lot 1 (murs orientés est-ouest, faute de rotation).

## 9. Hypothèses et risques

| Hypothèse ou risque | Statut | Parade |
|---|---|---|
| Le format d'identifiant (RG1) est le bon pour tous les objets | Vérifié sur les 59 identifiants actuels | Validation en jeu (US5) |
| Les tuiles (étals, portail) se placent comme les objets dans la slab | Probable : la slab ne distingue que l'UUID et la position | Validation en jeu sur les étals |
| Le jeu place l'origine de l'index à la position stockée (RG4) | Déduit des bornes de l'index | Validation en jeu ; sinon seule la formule de position change |
| L'emprise arrondie (RG4) suffit à éviter les chevauchements visibles | Tolérance de 0,15 case | Contrôle dans l'aperçu et en jeu |
| Hauteur des nénuphars sur l'eau | Inconnue | Validation en jeu ; `offset_z` à ajuster |
| L'index change à chaque mise à jour du jeu | Certain | Relancer l'importeur ; le test d'existence des UUID des thèmes et des 59 identifiants signale les disparitions |
| Redistribution des données Bouncyrock | Risque accepté par Antoine | Retirer `configs/taleweaver/` et `assets.json` sur demande |
| Taille du site (moteur WebAssembly déjà ~14 Mo) | +~1 Mio brut | Mesurer la taille compressée et le temps de lecture du catalogue au démarrage du moteur |
| Prompt trop long avec les thèmes | Faible (~60 objets) | Mesurer la taille du prompt avant et après |
| Le modèle invente des slugs hors thèmes | Probable | Retour à l'IA (US2) |

## 10. Découpage en lots

- **Lot 1 (cette spec)** : catalogue, résolution, encodage exact, emprise, 4 thèmes, 2 reliefs, aperçu, validation en jeu.
- **Lot 2** : motifs de placement dans l'IR (`row`, `ring`, `border`, `grid`) avec rotation ; objets composites (arbre de marais) ; tuiles de 0,25 case (pavés CobbleStone en surface) ; thème Sci-fi si besoin.
- **Lot 3** : assemblage de maisons simples à partir des kits muraux (rectangle, 4 murs, coins, toit).

## 11. Questions ouvertes, tranchées

1. **Où brancher la résolution catalogue** : la lecture de `assets.json` et `themes.json` dans `internal/catalogs` (il lit déjà depuis des octets) ; la résolution RG3 dans `internal/generator`, à côté de `aliases.go` ; l'émission via `placeBuildings` (RG4).
2. **Aperçu 2D** : contour de l'emprise réelle plus l'épingle actuelle au centre, pas de couleur par thème. La forme suffit à distinguer, ce qui respecte PRODUCT.md (pas la couleur seule). `HeightField` expose l'emprise de chaque objet placé.
3. **`kind` (tuile ou objet)** : interne. Le modèle reçoit le slug, l'emprise et le rôle.
4. **Prompt sans moteur** : rien de spécial. Le site sert `prompt-head.txt`, écrit au build depuis `nl.Catalog.PromptHead()` ; les thèmes y sont donc dès qu'ils sont dans le catalogue du prompt, et `TestPromptIsHeadPlusSceneLines` le garantit.

## Avant d'ouvrir la PR

`gofmt`, `go vet`, `go test -race ./...` verts en CI (natif et WebAssembly), golden inchangé, `TestSampleMapIsCurrent` vert ou mis à jour volontairement. Indiquer dans la description de PR : taille du prompt avant et après, taille du site (moteur brut et compressé) avant et après, temps de lecture du catalogue au démarrage. Mettre à jour `CLAUDE.md` (catalogue, thèmes, RG4/RG7).
