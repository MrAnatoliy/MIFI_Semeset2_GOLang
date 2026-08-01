.PHONY: up down build logs test tidy smoke psql

up:            ## Поднять весь стек (API + PostgreSQL + MailHog)
	docker compose up --build -d

down:          ## Остановить и удалить контейнеры
	docker compose down

clean:         ## Остановить и удалить данные БД
	docker compose down -v

build:
	docker compose build

logs:
	docker compose logs -f api

test:
	go test ./... -v

tidy:
	go mod tidy

smoke:         ## Прогнать сценарий проверки API
	./scripts/smoke.sh

psql:
	docker compose exec db psql -U bank -d bank
