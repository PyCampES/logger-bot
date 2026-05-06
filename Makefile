.PHONY: gen test build run e2e clean

PROTO_DIR := proto
OUT_DIR   := internal/whisper/pb

gen:
	@which protoc-gen-go        > /dev/null || (echo "Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; exit 1)
	@which protoc-gen-go-grpc   > /dev/null || (echo "Install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"; exit 1)
	protoc -I$(PROTO_DIR) \
	    --go_out=$(OUT_DIR)      --go_opt=paths=source_relative \
	    --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative \
	    $(PROTO_DIR)/transcribe.proto

test:
	go test ./... -count=1

build:
	go build -o bot ./cmd/bot

run:
	go run ./cmd/bot

e2e:
	./e2e/test_compose.sh

clean:
	rm -f bot $(OUT_DIR)/*.go