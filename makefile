APP_NAME = ./dist/nozomi
src = ./cmd/nozomi/main.go
BUILD_FLAGS = -tags goolm -ldflags="-w -s"

.PHONY: all
all: build

.PHONY: build
build:
	@echo "=> Building $(APP_NAME) ..."
	go build $(BUILD_FLAGS) -o $(APP_NAME) $(src)
	@echo "=> Done."