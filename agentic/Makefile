.PHONY: up down logs logs-alex status shell-alex

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

logs-alex:
	docker compose logs -f alex-developer

status:
	docker compose ps

shell-alex:
	docker compose exec alex-developer /bin/sh
