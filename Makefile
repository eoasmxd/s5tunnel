BIN_DIR := bin
LDFLAGS := -s -w

.PHONY: all build gui fmt release clean

all: fmt build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/s5ts ./cmd/s5ts
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/s5tc ./cmd/s5tc

gui:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS) -H=windowsgui" -o $(BIN_DIR)/s5tc-gui.exe ./cmd/s5tc-gui

fmt:
	go fmt ./...

release:
	bash ./package.sh

clean:
	rm -rf $(BIN_DIR) packages
