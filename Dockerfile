# L'éditeur dans un conteneur.
#
# L'image finale ne contient que le binaire et les données de démonstration :
# 9 Mio, sans shell, sans libc, sans gestionnaire de paquets. Le serveur n'a
# besoin ni de fichiers de police — le PDF utilise les polices standard Go —
# ni d'écriture sur disque : les brouillons vivent dans le navigateur.
#
#   docker build -t atscv .
#   docker run --rm -p 8080:8080 atscv
#
# L'instance est alors joignable depuis le réseau, donc sans authentification.
# C'est voulu, et c'est annoncé en clair au démarrage.

FROM golang:1.25-alpine AS build
WORKDIR /src

# Les dépendances d'abord, pour que la couche soit réutilisée tant que go.mod et
# go.sum ne bougent pas. Le module n'a qu'une dépendance, la bibliothèque PDF.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off rend le binaire statique, ce qui permet une image sans libc. -trimpath
# enlève les chemins de compilation, -s -w la table des symboles et DWARF.
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /atscv .

FROM scratch
COPY --from=build /atscv /atscv
# La démo sert de CV de départ quand on ouvre l'éditeur dans un navigateur.
COPY --from=build /src/data /data

# ATSCV_ADDR fixe l'adresse quand elle est présente. Sinon le binaire lit PORT,
# que tous les hébergeurs injectent, et écoute sur ce port : Render par défaut
# à 10000, Cloud Run à 8080. Sans les deux, il retombe sur 0.0.0.0:8080.
ENV ATSCV_ADDR=0.0.0.0:8080
ENV ATSCV_PUBLIC=true
ENV ATSCV_DEMO=/data/resume.demo.json
EXPOSE 8080

# /healthz existe déjà et ne touche ni au CV ni au disque.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD ["/atscv", "healthcheck"]

USER 65534:65534
ENTRYPOINT ["/atscv"]
CMD ["serve"]
