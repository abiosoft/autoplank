out=build
flags=-ldflags "-linkmode external -extldflags -static" -a
default_recipe: build

.PHONY:build
build:
	mkdir -p $(out)
	go build $(flags) -o $(out)/autoplank  main.go

.PHONY:install
install: build
	sudo cp $(out)/autoplank /usr/local/bin/