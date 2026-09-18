APP := gacha_appraisal
OUT := build/$(APP).wasm
PROD_OUT := production_build/$(APP).wasm
SRC := $(shell find . -name '*.go')

.PHONY: all build production_build clean test demo

build: $(OUT)

$(OUT): $(SRC)
	@mkdir -p $(@D)
	@echo "Building development WASM module..."
	tinygo build -o $@ -target=wasi .
	@ls -lh $@ | awk '{print "Binary size: " $$5}'

production_build: $(PROD_OUT)

$(PROD_OUT): $(SRC)
	@mkdir -p $(@D)
	@echo "Building production WASM module..."
	tinygo build -o $@ -opt=s -no-debug -target=wasi .
	@ls -lh $@ | awk '{print "Binary size: " $$5}'

test:
	go test ./app/...

demo:
	go run ./demo

clean:
	rm -rf build production_build

all: build production_build
