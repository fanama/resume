# atscv

Un générateur de CV lisible par les Applicant Tracking Systems (ATS), écrit en Go
sans aucune dépendance en dehors de la bibliothèque PDF.

Le CV est décrit dans un fichier JSON externe, puis rendu en `.docx`, `.pdf`,
`.adoc` et `.txt`. Les quatre sorties partagent le même flux de blocs, donc un
ATS qui extrait le texte du PDF lit exactement le même ordre que celui du DOCX.

## Installation

```sh
cd atscv
go build -o atscv .
```

## Utilisation

```sh
# Les trois formats principaux, dans un répertoire
./atscv build -i data/resume.fr.json -o dist/

# Un seul fichier, un seul format
./atscv build -i data/resume.fr.json -o dist/cv.pdf -f pdf

# Version anglaise à partir des données françaises
./atscv build -i data/resume.fr.json -lang en -o dist/ -name resume.en

# Ce qu'un ATS extrait, en clair
./atscv text -i data/resume.fr.json

# Le rapport ATS
./atscv lint -i data/resume.fr.json

# Validation des données et des dates, sans générer de fichier
./atscv check -i data/resume.fr.json
```

## Déploiement

L'outil se déploie en conteneur. Le `Dockerfile` est multi-étapes : l'image
finale est `scratch` plus le binaire, 9 Mio, sans shell ni libc, et le binaire
est compilé avec `CGO_ENABLED=0` pour que ce soit possible.

```sh
docker build -t atscv .
docker run --rm -p 8080:8080 atscv
```

Le serveur refuse volontairement toute adresse non-loopback, parce qu'il n'a ni
authentification ni limite de débit : `-public` (ou `ATSCV_PUBLIC=true`) est
l'exception explicite, et elle est annoncée en clair au démarrage. Les images
fournies le font déjà, donc une instance conteneurisée est publique dès le
premier démarrage.

Le port vient de `ATSCV_ADDR`, ou de `PORT` quand la première est absente. C'est
la convention de tous les hébergeurs : ils choisissent le port et vous le
passent, et écouter ailleurs revient à envoyer leur proxy dans le vide.
`PORT` ne décide que du port — sans `-public`, le serveur refuse quand même de
démarrer.

| Variable | Défaut | Rôle |
| --- | --- | --- |
| `ATSCV_ADDR` | `PORT`, sinon `127.0.0.1:8080` | adresse d'écoute |
| `PORT` | vide | port imposé par l'hébergeur, si `ATSCV_ADDR` est absente |
| `ATSCV_PUBLIC` | vide | écoute sur une adresse routable, sans authentification |
| `ATSCV_DEMO` | vide | CV proposé à l'ouverture d'un éditeur vide |
| `ATSCV_LANG` | `fr` | langue par défaut de l'éditeur |
| `ATSCV_ACCENT` | `1F3864` | accent des documents |
| `ATSCV_SIZE` | `10.5` | corps du texte, en points |
| `ATSCV_DATES` | `numeric` | format des dates |
| `ATSCV_FONT_DIR` | vide | répertoire de polices TrueType supplémentaires |

Chaque variable a son équivalent en drapeau (`-addr`, `-public`, `-demo`…), et
le drapeau l'emporte quand les deux sont posés.

La sonde de santé est la commande elle-même, l'image n'ayant ni shell ni curl :

```sh
atscv healthcheck   # interroge /healthz sur le port trouvé ci-dessus, sort 1 si l'instance ne répond pas
```

Un `SIGTERM` arrête le serveur en vidant les requêtes en cours, ce qui est ce
qu'envoient Fly.io, Render et Railway avant de tuer le processus.

**Avant de mettre une instance en ligne.** Elle rend le CV que l'on lui poste
et consomme le CPU de la machine, pour qui que l'atteigne. Il n'y a ni compte,
ni quota, ni limitation de débit. Deux options, dans cet ordre : la garder sur le
loopback et y accéder par un tunnel SSH, ou la mettre derrière votre propre
authentification. Un `healthcheck` vert ne dit rien de tout cela.

### Sur Render

Le palier gratuit donne 512 Mo, 0,1 CPU et 750 heures par mois, sans carte
bancaire. Le service s'endort après 15 minutes sans trafic et met une minute à
revenir, avec une page de chargement entre les deux.

1. Pousser le dépôt sur GitHub.
2. Render → **New → Web Service** → connecter le dépôt.
3. Environnement **Docker** : Render lit le `Dockerfile`.
4. Instance plan **Free**.
5. Health Check Path : `/healthz`.
6. Deploy.

`ATSCV_PUBLIC` et `ATSCV_DEMO` sont déjà dans l'image, il n'y a donc rien à
ajouter dans les variables d'environnement. Render produit
`atscv.onrender.com` en HTTPS.

Render injecte `PORT`, que l'image prend tel quel : c'est pourquoi le
`Dockerfile` porte un `ENV PORT=8080` et surtout **pas** de `ATSCV_ADDR`. Un
`ATSCV_ADDR` figé dans l'image gagnerait la priority sur le port injecté,
l'application écouterait sur 8080, le proxy chercherait 10000, et le
`healthcheck` resterait vert puisqu'il interroge la même mauvaise adresse.

Attention au trafic sortant : depuis avril 2026, le plan gratuit n'inclut que
5 Go par mois, contre 100 Go avant. Au-delà, 0,15 $/Go. Sans carte bancaire
Render suspend le service, avec une carte il facture. Vingt mille visites
entrent dans ces 5 Go, mais une API publique sans limitation de débit peut
faire déraper la consommation beaucoup plus vite qu'un site vitrine.

### Sur Cloud Run

```sh
gcloud run deploy atscv --source . --region us-central1 --allow-unauthenticated
```

Deux millions de requêtes et 180 000 vCPU-seconds sont offertes chaque mois, et
l'instance tombe à zéro quand personne n'est dessus. Le palier gratuit ne
s'applique qu'en `us-central1`, et il faut une carte bancaire : la facture reste
nulle tant que les quotas ne sont pas dépassés. Cloud Run envoie `PORT`, donc
rien d'autre à configurer.

### Les autres

Fly.io ne propose plus de palier gratuit aux nouveaux comptes depuis octobre
2024 : l'essai est de 2 heures machine ou 7 jours, puis facturation. Koyeb
offre un service gratuit mais exige une carte et une inscription qui part sur
le plan Pro payant, à dégrader à la main.

## L'éditeur web

```sh
# La page d'accueil et l'éditeur, sur http://127.0.0.1:8080
./atscv serve

# Autre port, autres options de rendu
./atscv serve -addr 127.0.0.1:9000 -accent 0A3B2C -size 11 -lang en
```

`atscv serve -demo data/resume.demo.json` ouvre l'éditeur sur une copie de la
démonstration au lieu d'un formulaire vide. Le serveur ne la sert qu'en lecture :
le navigateur la copie dans son `localStorage`, où elle devient un brouillon
ordinaire, détenu par le visiteur.

`atscv serve` sert deux pages : `/` est la page d'accueil du projet — ce que
fait l'outil, ses trois étapes, le linter, la pagination, l'installation — et
`/editor` est l'éditeur. Les deux viennent du même binaire et de la même
feuille de style, donc la vitrine et l'outil ne peuvent pas diverger. La page
d'accueil ne lit aucun CV et n'appelle aucun renderer.

`/editor` est un formulaire complet
construit à partir du schéma publié par le serveur, un aperçu qui se met à jour
à la frappe, le rapport ATS, le texte brut que l'ATS extrait, et quatre
téléchargements.

**Le serveur ne stocke rien.** Chaque session est un objet dans le
`localStorage` du navigateur (`atscv.sessions.v1`) ; le serveur ne reçoit que
le CV à rendre et ne le garde pas. Effacer les données du site efface les
brouillons : l'export JSON est la seule sauvegarde durable.

htmx est embarqué dans le binaire (`internal/web/static/htmx.min.js`), donc
l'éditeur fonctionne sans réseau.

### Options de `serve`

| Option | Défaut | Rôle |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8080` | adresse d'écoute ; **seul le loopback est accepté** |
| `-lang` | `fr` | `fr` ou `en` : titres de rubriques, libellés de l'éditeur, langue du document |
| `-dates` | `numeric` | même échelle que `build` |
| `-accent` | `1F3864` | couleur des titres, `RRGGBB` |
| `-size` | `10.5` | corps du texte, en points |
| `-rules` | **true** | filets sous les rubriques |
| `-font-dir` | — | dossier de polices TrueType pour le PDF |

### L'API

Toutes les routes de rendu sont en `POST` et attendent le CV en JSON, soit dans
le champ de formulaire `data`, soit directement en `application/json`. Le corps
est plafonné à 2 Mio.

| Route | Réponse |
| --- | --- |
| `GET /api/schema` | le schéma des champs, la source de vérité du formulaire |
| `POST /api/parse` | le CV validé et normalisé, ou `400` avec le champ fautif |
| `POST /api/lint` | le rapport en fragment HTML, ou en JSON avec `?format=json` |
| `POST /api/preview` | l'aperçu, rendu par le même `layout` que le PDF |
| `POST /api/text` | le texte brut, échappé, ou en JSON avec `?format=json` |
| `POST /api/render/{docx,pdf,adoc,txt}` | le fichier, en téléchargement |
| `GET /api/style.css` | l'accent et le corps, en CSS, pour une CSP stricte |
| `GET /` | la page d'accueil, en HTML, sans toucher au CV |
| `GET /editor` | l'éditeur |
| `GET /healthz` | `ok` |

```sh
# Valider un JSON sans navigateur
curl -s localhost:8080/api/parse -H 'Content-Type: application/json' --data-binary @data/resume.fr.json

# Le score ATS
curl -s 'localhost:8080/api/lint?format=json' --data-urlencode data@data/resume.fr.json

# Un PDF, nommé d'après la personne
curl -s -X POST localhost:8080/api/render/pdf \
  --data-urlencode data@data/resume.fr.json -o cv.pdf
```

### Sécurité

L'API n'a **ni authentification ni CSRF** : elle ne peut donc pas écouter autre
part que sur le loopback, et `serve` refuse de démarrer sur `0.0.0.0`. Le reste
est défensif : le modèle JSON est strict (un champ inconnu est une erreur, pas
un champ perdu), les chaînes sont échappées par `html/template` avant d'entrer
dans l'aperçu, les couleurs passent par le même validateur que l'option
`-accent`, et la politique de sécurité interdit tout script et tout style
inline, tout chargement distant et tout framing.

Ce que le serveur ne fait pas, et qu'il faut savoir : la Tierce partie n'a
aucun moyen de distinguer un CV d'un autre. Les brouillons vivent dans le
navigateur, donc sur une machine partagée ils sont accessibles à quiconque ouvre
le même profil.


### Options de `build`

| Option | Défaut | Rôle |
| --- | --- | --- |
| `-i`, `-in` | — | fichier JSON d'entrée, obligatoire |
| `-o` | `.` | répertoire de sortie, ou fichier si `-f` ne liste qu'un format |
| `-f` | `docx,pdf,adoc` | `docx`, `pdf`, `adoc`, `txt` |
| `-lang` | celui du JSON | `fr` ou `en`, pour les titres de rubriques |
| `-name` | nom du JSON | nom de base des fichiers générés |
| `-dates` | `numeric` | `numeric` (`03/2022 - 08/2022`), `month` (`mars 2022 - août 2022`), `year` |
| `-ascii` | false | supprime les accents pour qu'une recherche par mot-clé ne rate jamais |
| `-rules` | **true** | filets sous les titres de rubriques et sous le bloc d'identité, `-rules=false` pour les supprimer |
| `-accent` | `1F3864` | couleur des titres, du titre de poste et des libellés, `RRGGBB` |
| `-font` | `Calibri` (docx), `Arial` (pdf) | police |
| `-size` | `10.5` | corps du texte, en points ; toute l'échelle typographique suit |
| `-font-dir`, `-font-file`, `-bold-font-file` | — | polices TrueType explicites pour le PDF |
| `-q` | false | n'affiche rien si tout s'est bien passé |

## Le design

Toute l'identité visuelle tient en cinq décisions, et **aucune n'ajoute de
structure** : un ATS ne voit ni une couleur, ni une bordure, ni un interligne.

1. **Une seule échelle typographique.** Le corps (10,5 pt) est la référence ; le
   nom est à +4,5 pt, le titre de poste et les rubriques à +1 pt, les
   méta-informations à −1 pt, les contacts à −1,5 pt. `-size` redimensionne
   donc l'ensemble du document, pas seulement le texte.
2. **Un accent, un seul ton.** `#1F3864` sur le nom, les titres de rubriques et
   les libellés (`Technologies :`, `Équipe :`). Le reste est noir ou gris
   (`#404040` pour les dates et organisations, `#595959` pour les contacts,
   `#8A8A8A` pour le tiret des puces) : l'œil va droit aux mots-clés.
3. **Des filets, pas des cadres.** Une bordure de paragraphe sous chaque titre de
   rubrique, plus une sous le bloc d'identité. C'est un `w:pBdr` en OOXML et un
   `Line()` en PDF — jamais un tableau, une zone de texte ou une forme, que le
   plus petit parseur prend pour un tableau de mise en page.
4. **Un rythme vertical régulier.** 6 mm avant une rubrique, 2,2 mm après, 3 mm
   entre deux entrées, interligne de 1,25, marges de 14 mm. Une page A4 contient
   58 lignes à 10,5 pt : c'est la contrainte qui décide de la longueur du CV,
   pas le goût.
5. **Une hiérarchie qui survit à l'extraction.** Le tiret d'une réalisation est
   aligné à 6 mm avec un retrait négatif, donc une ligne qui se replie reste
   alignée sur le texte et pas sur le tiret — et le parseur voit une seule
   ligne, comme dans le `.docx`.
6. **Le `.docx` est plus serré que le `.pdf`.** Le Word tient l'interligne à
   1,1 là où le PDF est à 1,25, parce que c'est le Word qui part dans les
   moteurs ATS. Les deux ont les mêmes marges, la même police relative et le
   même accent.

```
Rakotoasimbola Fananamperantsoa          <- 15 pt, gras, accent, interlettrage 1 pt
Développeur Fullstack spécialisé en IA…   <- 11,5 pt, gris
Paris, France | f.rakotoasimbola@…        <- 9 pt, gris clair
──────────────────────────────           <- filet sous l'identité
PROFIL                                   <- 11,5 pt, gras, capitales, accent
──────────────────────────────           <- filet sous la rubrique
```

## Pagination

Le PDF mesure chaque bloc **avant** de le dessiner : une coupure décidée après
le texte a débordé, c'est un orphelin que le lecteur voit tout de suite. Trois
règles, chacune testée :

- **jamais un titre de rubrique seul en bas de page** — le titre, son filet et la
  première ligne de ce qu'il annonce vont ensemble, sinon la page suivante
  commence par un mot qui ne veut rien dire seul ;
- **une entrée entière reste sur une page** quand elle y tient — une expérience,
  une formation, une activité ne se coupe pas en deux ;
- **une entrée plus haute qu'une page se coupe quand même**, mais son titre, ses
  dates et la première ligne de son corps restent ensemble.

Le `.docx` applique les mêmes trois règles avec les propriétés de paragraphe que
Word comprend : `keepNext` sur le nom, le titre, chaque rubrique, chaque titre
d'entrée et chaque méta, plus `keepLines` sur les titres, et le corps de chaque
entrée chaîné jusqu'à son dernier paragraphe. Le dernier paragraphe d'une entrée
ne porte **pas** `keepNext` : sinon deux entrées seraient liées et un document de
petites entrées se pousserait vers le bas de la page pour rester avec lui-même.
Quand une entrée dépasse la hauteur d'une page, Word ignore `keepNext` et coupe
là où il peut — le même repli que le PDF.

Aucune pagination ne comporte ni pied de page ni numéro : c'est du bruit pour un
parseur comme pour un recruteur.

Le nombre de pages est **mesuré**, jamais estimé : `check` et `build` font
tourner le vrai moteur avec la vraie police, et disent ce qu'il faudrait couper.

```sh
$ atscv check -i data/resume.fr.json
ok: 613 words, 7 entries, 28 skills, 2 pages, the last one 64% full: it would take 173 mm of content to fit on one
```

Passer de deux pages à une seule n'est donc pas un réglage de mise en page :
173 mm, c'est une rubrique entière. Les leviers réels, dans l'ordre de ce qu'ils
coûtent le moins : raccourcir le profil, réduire les réalisations à deux par
poste, fusionner les compétences proches, ou dropper les activités — c'est la
seule section qu'un recruteur ATS regarde en dernier.

## Pourquoi ce CV est lisible par un ATS

Chaque renderer respecte les mêmes règles, et `internal/render/docx/docx_test.go`
les vérifie à chaque `go test` :

- une seule colonne, dans l'ordre : identité, profil, expérience, formation,
  compétences, certifications, projets, activités, langues ;
- aucun tableau, aucune zone de texte, aucun cadre, aucune image, aucun en-tête,
  aucun pied de page, aucun numéro de page ;
- des puces Word natives (`numbering.xml`), pas un tiret tapé ;
- aucun lien caché : les URL sont écrites en texte brut, `https://` retiré ;
- une police standard, A4, des marges de 14 mm, des styles nommés
  (`ATSName`, `ATSSection`, `ATSBullet`...) pour que le document reste
  restylable depuis Word ;
- en PDF, la police TrueType est embarquée avec sa table `ToUnicode`, sinon les
  accents ressortent en charabia à l'extraction ; sans police TrueType disponible,
  le texte est écrit en cp1252, ce qui couvre le français.

## Linter ATS

`atscv lint` note ce qu'un parser perd ou ce qui coûte des points :

```text
warning  contact.phone         header         no phone number
warning  dates.start           activities[0]   the period has no start date
info     entry.metrics         experience[2]   no line of Cliris / Dasia carries a number

readability 81/100 | ~2 page(s) | 647 words | 7 entries | 15 achievement lines, 3 with a figure | 28 skills
```

- `error` : l'information sera perdue ou illisible à l'extraction, `lint` sort en 1 ;
- `warning` : un filtre, une recherche par mot-clé ou un recruteur perdra un point ;
- `info` : une suggestion.

`check` ne sort en 1 que sur les `error`, ce qui permet de l'utiliser dans un
Makefile ou une CI.

## Format des données

Un seul champ par information, et uniquement des faits : le générateur n'invente
aucune date, aucun chiffre. Une clé mal orthographiée est une erreur, pas un
silence : `model.Parse` refuse un champ inconnu au lieu de perdre une rubrique.

```json
{
  "lang": "fr",
  "name": "Rakotoasimbola Fananamperantsoa",
  "headline": "Développeur Fullstack spécialisé en IA générative (LLM)",
  "contact": {
    "email": "f.rakotoasimbola@gmail.com",
    "phone": "",
    "city": "Paris",
    "country": "France",
    "links": [{ "label": "GitHub", "url": "https://github.com/fanama" }]
  },
  "summary": "...",
  "experience": [
    {
      "title": "Développeur Fullstack LLM",
      "company": "SFR",
      "location": "Paris",
      "start": "2023",
      "end": "present",
      "summary": "...",
      "highlights": ["..."],
      "team": "Product Manager, Product Owner et trois développeurs",
      "stack": ["React", "NodeJS", "MongoDB"]
    }
  ],
  "education": [
    { "degree": "...", "school": "...", "location": "...", "start": "2017", "end": "2020" }
  ],
  "skills": [{ "category": "Langages", "items": ["Go", "Python"] }],
  "certifications": [{ "name": "...", "issuer": "...", "date": "2024", "id": "..." }],
  "projects": [{ "name": "...", "url": "...", "start": "...", "end": "...", "summary": "...", "highlights": ["..."], "technologies": ["..."] }],
  "activities": [{ "name": "...", "organization": "...", "location": "...", "start": "...", "end": "...", "summary": "...", "highlights": ["..."] }],
  "languages": [{ "name": "Français", "level": "Langue maternelle" }]
}
```

Les dates acceptent `2023`, `2023-01`, `01/2023`, `janvier 2023` et `present` /
`aujourd'hui`. Une date réduite à l'année reste une année : le générateur n'ajoute
jamais un mois qui n'a pas été écrit.

## Tests

```sh
go test ./...
go vet ./...
```

La suite couvre le schéma JSON, le formatage des dates, l'ordre des rubriques, la
pliage des accents, l'interdiction des structures hostiles dans le DOCX, la police
et le repli cp1252 du PDF, les quatre renderers, la CLI de bout en bout et
l'API web : routes, codes de statut, en-têtes, échappement, et un garde-fou qui
échoue si un champ du modèle n'a pas d'entrée dans le formulaire.

Le test de l'éditeur (`TestEditorDrivesTheForm`) exécute `app.js` dans un DOM
miniature, sans navigateur : il vérifie qu'une frappe écrit la bonne clé du
modèle, qu'un bloc unique n'offre ni suppression ni duplication, qu'une session
survit à un rechargement, et que le téléchargement porte le bon brouillon. Il
est ignoré si `node` est absent.

Le `.docx` produit est reproductible bit à bit, ce qui permet de le versionner.

## Rendu AsciiDoc

Le renderer écrit un `.adoc` mono-colonne, sans table ni image, que le
générateur Go produit :

```sh
./atscv build -i data/resume.fr.json -o dist/ -f adoc
asciidoctor -b pdf -o dist/resume.fr.pdf dist/resume.fr.adoc
```

L'ancien outillage (`resume.adoc`, `components/*.adoc` et ses rendus) a été
supprimé : les données vivent désormais dans `data/resume.*.json`. L'historique
git les conserve, `git show HEAD:components/skills.adoc` par exemple.
