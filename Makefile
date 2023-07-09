.PHONY: all install clean

all:
	@go build -o bin/cert-monitor cmd/cert-monitor/main.go

install:
	@cp bin/cert-monitor /usr/local/bin/cert-monitor

clean:
	@rm -f bin/cert-monitor /usr/local/bin/cert-monitor

