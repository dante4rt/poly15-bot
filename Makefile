.PHONY: build run run-dry scan approve balance test clean docker-build docker-run docker-logs docker-stop sports sports-dry blackswan blackswan-dry weather weather-dry derive-creds

# Local development
build:
	@mkdir -p bin
	go build -o bin/sniper ./cmd/sniper
	go build -o bin/scanner ./cmd/scanner
	go build -o bin/approve ./cmd/approve
	go build -o bin/balance ./cmd/balance
	go build -o bin/sports ./cmd/sports
	go build -o bin/blackswan ./cmd/blackswan
	go build -o bin/weather ./cmd/weather
	go build -o bin/derive-creds ./cmd/derive-creds

run:
	./bin/sniper

run-dry:
	DRY_RUN=true ./bin/sniper

sports:
	./bin/sports

sports-dry:
	DRY_RUN=true ./bin/sports

blackswan:
	./bin/blackswan

blackswan-dry:
	DRY_RUN=true ./bin/blackswan

weather:
	./bin/weather

weather-dry:
	DRY_RUN=true ./bin/weather

scan:
	./bin/scanner

approve:
	./bin/approve

balance:
	./bin/balance

derive-creds:
	./bin/derive-creds

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
