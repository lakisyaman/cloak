.PHONY: test test-integration

test:
	go test ./...

test-integration:
	go test -tags=integration ./test/integration -count=1 -v
