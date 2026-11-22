# Analyse des slabs de la communauté : les ruines

*22 novembre 2026. Étude de 107 slabs choisis par Antoine sur TalesTavern.*

Les ruines générées jusqu'ici n'étaient que de gros rochers : le terrain
« ruines » dispersait une falaise de 4,6 cases et un rocher sur 30 % de ses
cases. Pour faire mieux, on a regardé comment les joueurs construisent leurs
ruines dans TaleSpire, et on a appris au générateur à faire de même, avec les
pièces du jeu.

## Ce qui a été étudié, et ce qui ne l'a pas été

- **Le corpus.** 107 slabs : villages (50), maisons (22), nature (22), grottes
  (20), ruines (16), forteresses (12), marais (16 en terrain), cimetières,
  ponts, etc.
- **Décodage.** 95 slabs ont été décodés, soit environ 220 000 pièces. Les 12
  autres utilisent l'ancien format de TaleSpire (version 1), que le décodeur ne
  lit pas. Environ 1 % des pièces n'existent plus dans le catalogue actuel du
  jeu.
- **Licences.** 58 slabs n'indiquent aucune licence, les autres sont sous
  licences Creative Commons, souvent « pas d'usage commercial ». Les slabs
  n'ont donc été qu'**étudiés** : aucun n'est copié dans le dépôt ni dans le
  site. Le générateur construit ses propres ruines, avec les pièces du jeu et
  les règles ci-dessous.

## Les ruines étudiées

16 slabs, dont les plus utiles : *Monster Lair – Forest Town Ruin* (10595),
*a selection of ruined houses and shrines* (654), *Ruined City section / Snow*
(1878), *Hazardous ruins 1 et 2* (2439, 2496), *Ruined Castle* (6408), *Ruined
Tower* (5395), *Old Ruin / Ruine ancienne* (3115), *Swamp Ruine* (9388).

### Les pièces utilisées

- **Murs** : la gamme « Castle Ruins » du jeu, surtout le mur de 1 et de 2 cases
  (`castle_ruins_wall_01`, `_02`), un demi-carreau d'épaisseur et deux cases de
  haut. Plus rarement : soubassements, angles, arches, piliers, créneaux, murs à
  fenêtre écroulée.
- **Sols** : les dalles « Castle Ruins » (1×1 et 2×2), parfois du plancher, posés
  **sur** l'herbe ou la terre, pas à leur place.
- **Gravats** : le tas de pierres `castle_ruins_rockpile` est de loin la pièce la
  plus utilisée (jusqu'à 5 000 dans un seul slab), plus des tas de rochers, des
  graviers épars, des tas de terre.
- **Végétation** : arbres feuillus, fougères, buissons morts, racines, lianes.

### Comment elles sont assemblées

- **Des bâtiments rectangulaires**, de 6 × 6 à environ 10 × 12 cases, souvent
  plusieurs côte à côte avec un passage entre eux (un village en ruine).
- **Une seule rangée de murs**, posée sur la moitié intérieure de l'anneau de
  cases qui entoure le sol. Les murs sont tournés selon leur côté (ouest 0°,
  nord 90°, est 180°, sud 270°).
- **Des pans courts** : la plupart des segments de mur font 2 à 6 cases, coupés
  par une porte et des brèches.
- **Le secret du mur écroulé** : environ la moitié des morceaux de mur sont posés
  sur le sol ; les autres sont **enfoncés** d'un à trois demi-carreaux dans le
  sol, ce qui donne la ligne de faîte irrégulière. Quelques morceaux portent une
  seconde rangée, elle aussi enfoncée : ce sont les derniers pans hauts.
- **Des gravats empilés aux angles**, là où deux murs se rejoignent (2 à 4 tas
  l'un sur l'autre), d'autres dans les brèches et à l'intérieur.
- **Un sol cassé** : l'herbe perce entre les dalles, et des plantes (et parfois
  un arbre) poussent là où le sol a disparu.

Un détail technique, mesuré sur ces slabs : la position d'une pièce tournée est
le coin de son emprise une fois tournée. Une dalle tournée d'un quart de tour
garde donc la même position.

## Ce que fait le générateur maintenant

Dans une zone « ruines », le générateur construit des bâtiments en ruine selon
ces règles : sol de dalles cassé (environ 72 % des cases), murs avec une porte
et des brèches, morceaux enfoncés de 0 à 3 demi-carreaux (la moitié posés au
sol), quelques pans plus hauts, gravats aux angles, dans les brèches et à
l'intérieur, plantes là où le sol manque, un arbre dans certains grands
bâtiments. Les bâtiments couvrent environ 40 % de la zone ; le reste forme les
passages, avec du gravier, des gravats et quelques buissons. L'aperçu dessine
les murs à l'échelle.

Les bâtiments évitent les chemins, l'eau, les objets et les points d'intérêt
placés à la main (un autel au milieu des ruines reste à sa place).

## Pistes pour la suite

- **Plus de pièces** : arches au-dessus des portes, piliers isolés, murs à
  fenêtre écroulée, soubassements, lianes qui pendent des murs, poutres
  tombées.
- **Des cloisons** : certaines maisons ont deux ou trois pièces.
- **Les autres catégories du corpus** : marais (huttes sur pilotis, tourbières),
  grottes, cimetières et villages pourraient être étudiés de la même façon pour
  de nouveaux thèmes.
