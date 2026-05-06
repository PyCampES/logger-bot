# --- build stage ---
FROM golang:1.26-bookworm AS build

# Install protoc + plugins so codegen happens in the build
RUN apt-get update \
 && apt-get install -y --no-install-recommends protobuf-compiler make \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Need proto/ + Makefile + source
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

ENV WHISPER_ADDR=whisper:50051
ENV DB_PATH=/data/log.db

COPY --from=build /out/bot /usr/local/bin/bot
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/bot"]