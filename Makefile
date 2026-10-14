MOCKERY_VERSION := v3.8.1

.PHONY: test fmt tidy mocks install

test:
	go test ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

mocks:
	go run github.com/vektra/mockery/v3@$(MOCKERY_VERSION)

install:
	go install ./cmd/ero
