.PHONY: build run run-dry scan approve test clean docker-build docker-run docker-logs docker-stop

# Local development
build:
	@mkdir -p bin
	go build -o bin/sniper ./cmd/sniper
	go build -o bin/scanner ./cmd/scanner
	go build -o bin/approve ./cmd/approve

run:
	./bin/sniper

run-dry:
	DRY_RUN=true ./bin/sniper

scan:
	./bin/scanner

approve:
	./bin/approve

test:
	go test -v ./...

clean:
	rm -rf bin/

# Docker commands
docker-build:
	docker build -t polymarket-sniper .

docker-run:
	docker-compose up -d sniper

docker-scan:
	docker-compose run --rm scanner

docker-approve:
	docker-compose run --rm approve

docker-logs:
	docker-compose logs -f

docker-stop:
	docker-compose down

docker-clean:
	docker-compose down --rmi local -v
