FROM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG GIT_SHA
WORKDIR /src
COPY app/server/go.mod app/server/go.sum ./
RUN go mod download
COPY app/server/*.go ./
RUN test "${#GIT_SHA}" -eq 40 && printf '%s\n' "$GIT_SHA" | grep -Eq '^[0-9a-f]{40}$' && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.gitSHA=$GIT_SHA" -o /out/site .
RUN apk add --no-cache ca-certificates

FROM scratch
WORKDIR /site
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/site /site/server
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/site/server"]
CMD ["serve"]
