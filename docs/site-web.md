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

À chaque push sur `main`, le workflow GitHub **Website** reconstruit le site,
teste le moteur dans son vrai runtime WebAssembly, puis le publie sur Cloudflare
Pages. Sur une PR, il construit et teste sans publier.

## Mise en place (une seule fois)

> Rédigé sans avoir pu ouvrir le site de Cloudflare depuis l'environnement de
> développement : un libellé de menu peut avoir légèrement changé.

### 1. Créer un jeton d'API Cloudflare

1. Tableau de bord Cloudflare → icône de profil → **My Profile** → **API Tokens**
   → **Create Token** → **Create Custom Token**.
2. Nom : `Le Cartographe — GitHub`.
3. Permissions : **Account** · **Cloudflare Pages** · **Edit**. Rien d'autre.
4. **Continue to summary** → **Create Token**, puis copiez le jeton (il n'est
   affiché qu'une fois).

Notez aussi votre **Account ID** : **Workers & Pages** → colonne de droite.

### 2. Donner ces deux valeurs à GitHub

Dépôt GitHub → **Settings** → **Secrets and variables** → **Actions** →
**New repository secret**, deux fois :

| Nom                     | Valeur              |
|-------------------------|---------------------|
| `CLOUDFLARE_API_TOKEN`  | le jeton de l'étape 1 |
| `CLOUDFLARE_ACCOUNT_ID` | l'Account ID        |

### 3. Publier

Onglet **Actions** → **Website** → **Run workflow** (ou simplement un push sur
`main`). Le premier déploiement crée le projet Pages `le-cartographe`. Le site
est alors en ligne sur `https://le-cartographe.pages.dev` (Cloudflare ajoute un
suffixe si ce nom est déjà pris : l'adresse exacte s'affiche à la fin du job).

### 4. Brancher votre sous-domaine

Cloudflare → **Workers & Pages** → **le-cartographe** → **Custom domains** →
**Set up a custom domain** → saisissez le sous-domaine (par exemple
`cartographe.votredomaine.fr`) → **Activate domain**.

Comme le DNS du domaine est déjà chez Cloudflare, l'enregistrement DNS et le
certificat HTTPS sont créés automatiquement. Comptez quelques minutes.

## Sans attendre GitHub : glisser-déposer

Pour une mise en ligne immédiate, sans jeton : construisez le site
(`scripts/build-web.sh`, ou téléchargez l'artefact **site** d'un job **Website**),
puis Cloudflare → **Workers & Pages** → **Create** → **Pages** → **Upload
assets**, nommez le projet `le-cartographe` et déposez le dossier `dist/web`.

Gardez bien ce nom : le workflow GitHub publiera ensuite dans le même projet.

## Tester localement

```sh
scripts/build-web.sh
cd dist/web && python3 -m http.server 8000   # puis http://localhost:8000
```

Le site doit être servi en HTTP : ouvrir `index.html` directement depuis le
disque ne fonctionne pas (le navigateur refuse alors de charger le moteur).
