.PHONY: build test run-ingest run-worker run-query run-ml tidy clean

build:
	go build -o bin/ingest cmd/ingest/main.go
	go build -o bin/worker cmd/worker/main.go
	go build -o bin/query cmd/query/main.go
	go build -o bin/ml-worker cmd/ml-worker/main.go

test:
	go test -v ./...

run-ingest:
	go run cmd/ingest/main.go

run-worker:
	WORKER_ID=worker-1 go run cmd/worker/main.go

run-query:
	go run cmd/query/main.go

run-ml:
	go run cmd/ml-worker/main.go

tidy:
	go mod tidy

clean:
	rm -rf bin/
