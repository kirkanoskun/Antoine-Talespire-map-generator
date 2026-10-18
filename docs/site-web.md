# Le Cartographe sur le web

Le site public est une version **100 % statique** de l'application : le moteur de
génération (le même code Go que l'app de bureau) est compilé en WebAssembly et
tourne **dans le navigateur du visiteur**. Il n'y a donc aucun serveur à faire
tourner, à payer ni à surveiller ; Cloudflare se contente de servir des fichiers.

Ce que le site propose : décrire une scène → copier le prompt (v1.03) → coller le
JSON renvoyé par son IA → aperçu 2D + code TaleSpire. La génération directe par
IA n'est **pas** branchée : elle tournerait avec votre clé Anthropic, aux frais de
chaque visiteur. Le bouton « Quitter » est masqué (il n'y a rien à arrêter).

## Comment c'est construit

```
scripts/build-web.sh     → dist/web/  (le site complet)
  app.wasm               le moteur (cmd/web), ~13 Mo, ~2,8 Mo compressé
  wasm_exec.js           le runtime Go pour navigateur
  cartographe.js         le pont : envoie les appels /api/* de la page au moteur
  index.html, static/    l'interface de l'app de bureau, inchangée
```

Le site ne charge rien depuis un service tiers : les polices (Cinzel, Work Sans,
licence OFL) sont hébergées dans `static/fonts/`, et un test
(`TestPagesLoadOnlyTheirOwnResources`) échoue si une page se remet à appeler un
autre domaine. Les mentions légales (`static/mentions-legales.html`) sont liées
en bas de chaque page ; elles affirment cette absence de collecte, donc
mettez-les à jour si un jour le site ajoute une mesure d'audience ou un service
externe.

À chaque push sur `main`, le workflow GitHub **Website** reconstruit le site,
teste le moteur dans son vrai runtime WebAssembly, puis le publie sur Cloudflare.
Sur une PR, il construit et teste sans publier.

## Hébergement : un « Worker » sans code, gratuit

Le site est publié comme un **Worker à fichiers statiques** (`wrangler.jsonc`),
la formule que Cloudflare propose désormais à la place de Pages (que son
tableau de bord marque « legacy »). Aucun code ne tourne chez Cloudflare : il
sert des fichiers, et c'est **gratuit et illimité sur le plan Free**. Les
limites du plan Workers Free (100 000 requêtes par jour, 10 ms de calcul) ne
concernent que le code exécuté chez Cloudflare, donc pas ce site. Seule limite
pertinente : 25 Mo par fichier ; le plus gros, `app.wasm`, fait ~13,5 Mo.

Le nom du Worker est **`le-cartographe`**, partout : dans `wrangler.jsonc`, à la
mise en ligne manuelle et dans le workflow. Un autre nom créerait un second
site au lieu de mettre à jour le premier.

## Mise en ligne

### 1. Première publication (glisser-déposer, 2 minutes)

1. Récupérez le site construit : `scripts/build-web.sh` produit `dist/web/`
   (ou téléchargez l'artefact **site** d'un job **Website** sur GitHub).
2. Cloudflare → **Workers & Pages** → **Create** → **Upload your static files**.
3. Nom : `le-cartographe`. Déposez le **contenu** de `dist/web/` (`index.html`,
   `app.wasm`, le dossier `static`…), pas le dossier parent → **Deploy**.
4. Le site répond sur `https://le-cartographe.<votre-compte>.workers.dev`.

### 2. Brancher le sous-domaine `cartographe.antoinedebaets.com`

1. Vérifiez que le domaine est géré par Cloudflare : la page d'accueil du
   tableau de bord liste `antoinedebaets.com` avec le statut **Active**.
2. **Workers & Pages** → **le-cartographe** → **Settings** →
   **Domains & Routes** → **Add** → **Custom domain**.
3. Saisissez `cartographe.antoinedebaets.com` → **Add domain**.

Cloudflare crée lui-même l'enregistrement DNS et le certificat HTTPS ; comptez
quelques minutes. **Ne créez pas d'enregistrement DNS à la main** pour ce nom :
un enregistrement existant bloque l'ajout (« already has externally managed DNS
records »). Si cela arrive, supprimez-le dans l'onglet **DNS** du domaine et
recommencez l'étape 3.

L'adresse en `workers.dev` reste accessible ; c'est le même site.

### 3. Mises à jour automatiques depuis GitHub (facultatif)

Sans cette étape, chaque nouvelle version se publie en refaisant le
glisser-déposer. Avec, chaque push sur `main` est publié tout seul.

1. **Jeton d'API** : icône de profil → **My Profile** → **API Tokens** →
   **Create Token** → modèle **Edit Cloudflare Workers** → **Use template**.
   Dans **Account Resources**, choisissez votre compte ; dans
   **Zone Resources**, `antoinedebaets.com` suffit. → **Continue to summary**
   → **Create Token**, puis copiez le jeton (il n'est affiché qu'une fois).
2. **Account ID** : **Workers & Pages** → colonne de droite (c'est aussi la
   longue suite de caractères dans l'adresse du tableau de bord).
3. Dépôt GitHub → **Settings** → **Secrets and variables** → **Actions** →
   **New repository secret**, deux fois :

   | Nom                     | Valeur                |
   |-------------------------|-----------------------|
   | `CLOUDFLARE_API_TOKEN`  | le jeton              |
   | `CLOUDFLARE_ACCOUNT_ID` | l'Account ID          |

4. Onglet **Actions** → **Website** → **Run workflow** pour vérifier : le job
   **deploy** doit finir en vert et met à jour le Worker `le-cartographe`. Le
   sous-domaine reste branché d'un déploiement à l'autre.

## Tester localement

```sh
scripts/build-web.sh
npx wrangler dev        # le vrai moteur de Cloudflare, sur http://localhost:8787
# ou, sans Node :
cd dist/web && python3 -m http.server 8000
```

Le site doit être servi en HTTP : ouvrir `index.html` directement depuis le
disque ne fonctionne pas (le navigateur refuse alors de charger le moteur).
