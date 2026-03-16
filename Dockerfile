# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

# CA roots are copied into the scratch runtime image so HTTPS works there too.
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
	go build \
	-trimpath \
	-ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
	-o /out/paserati \
	./cmd/paserati

FROM scratch AS runtime

WORKDIR /work

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/paserati /paserati

USER 65532:65532

ENTRYPOINT ["/paserati"]
