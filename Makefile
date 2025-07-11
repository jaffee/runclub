.PHONY: build clean

build:
	go build -o runclub main.go

clean:
	rm -f runclub