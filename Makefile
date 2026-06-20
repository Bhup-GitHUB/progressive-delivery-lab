COMPOSE=docker-compose

.PHONY: up down logs demo-good demo-bad rollback enable-flag

up:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

enable-flag:
	curl -s -X POST http://localhost:8081/flags/fraud-model -H 'Content-Type: application/json' -d '{"enabled":true,"rolloutPercent":1}'

demo-good:
	$(COMPOSE) up --build -d
	curl -s -X POST http://localhost:8081/flags/fraud-model -H 'Content-Type: application/json' -d '{"enabled":true,"rolloutPercent":1}'
	$(COMPOSE) run --rm loadgen -target http://router:8080 -mode normal -duration 90s -rps 25

demo-bad:
	CANARY_FRAUD_MODEL_LATENCY_MS=650 CANARY_FRAUD_MODEL_ERROR_RATE=0.08 $(COMPOSE) up --build -d
	curl -s -X POST http://localhost:8081/flags/fraud-model -H 'Content-Type: application/json' -d '{"enabled":true,"rolloutPercent":1}'
	$(COMPOSE) run --rm loadgen -target http://router:8080 -mode bad -duration 90s -rps 35

rollback:
	curl -s -X POST http://localhost:8080/rollback
	curl -s -X POST http://localhost:8081/flags/fraud-model/kill
