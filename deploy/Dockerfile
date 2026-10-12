FROM golang:1.27-alpine AS build
RUN apk add --no-cache make gzip
WORKDIR /src
COPY go.mod go.sum ./
COPY tools/gendocs/go.mod tools/gendocs/go.sum tools/gendocs/
RUN go mod download && cd tools/gendocs && go mod download
COPY . .
RUN make site-headless
RUN find site -type f \( -name '*.js' -o -name '*.css' -o -name '*.html' -o -name '*.json' -o -name '*.svg' \) -exec gzip -9 -k -f {} \;

FROM nginx:1.27-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/site /usr/share/nginx/html
HEALTHCHECK --interval=10s --timeout=3s CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
