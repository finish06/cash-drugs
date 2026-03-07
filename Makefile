.PHONY: test test-unit test-integration test-mongo-up test-mongo-down

# Run unit tests only (no MongoDB required)
test-unit:
	go test ./... -short -count=1

# Start test MongoDB
test-mongo-up:
	docker compose -f docker-compose.test.yml up -d

# Stop test MongoDB
test-mongo-down:
	docker compose -f docker-compose.test.yml down

# Run all tests including integration (requires MongoDB on port 27018)
test-integration: test-mongo-up
	MONGO_URI="mongodb://localhost:27018/drugs_test" go test ./... -count=1

# Run all tests with coverage report
test-coverage: test-mongo-up
	MONGO_URI="mongodb://localhost:27018/drugs_test" go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	@echo "HTML report: go tool cover -html=coverage.out"

# Run all tests (alias)
test: test-unit
