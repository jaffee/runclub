.PHONY: run build clean

run:
	go run main.go

build:
	go build -o qrserver main.go

clean:
	rm -f qrserver