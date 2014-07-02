
build:
	mkdir -p stage
	go build -o stage/docksul
	docker build -t docksul .