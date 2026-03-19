.PHONY: up down logs status \
	logs-alex shell-alex \
	logs-jeanpierre shell-jeanpierre \
	logs-katrin shell-katrin \
	logs-yuki shell-yuki \
	logs-maria shell-maria \
	logs-lisa shell-lisa \
	logs-taylor shell-taylor \
	logs-all-agents

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

status:
	docker compose ps

# --- Per-agent targets ---

logs-alex:
	docker compose logs -f alex-developer

shell-alex:
	docker compose exec alex-developer /bin/sh

logs-jeanpierre:
	docker compose logs -f jeanpierre-fr

shell-jeanpierre:
	docker compose exec jeanpierre-fr /bin/sh

logs-katrin:
	docker compose logs -f katrin-de

shell-katrin:
	docker compose exec katrin-de /bin/sh

logs-yuki:
	docker compose logs -f yuki-ja

shell-yuki:
	docker compose exec yuki-ja /bin/sh

logs-maria:
	docker compose logs -f maria-brand

shell-maria:
	docker compose exec maria-brand /bin/sh

logs-lisa:
	docker compose logs -f lisa-pm

shell-lisa:
	docker compose exec lisa-pm /bin/sh

logs-taylor:
	docker compose logs -f taylor-qa

shell-taylor:
	docker compose exec taylor-qa /bin/sh

logs-all-agents:
	docker compose logs -f alex-developer jeanpierre-fr katrin-de yuki-ja maria-brand lisa-pm taylor-qa
