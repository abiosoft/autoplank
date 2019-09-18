default_recipe: build

build:
	go build -o autoplank main.go

install: build
	sudo cp autoplank /usr/local/bin/