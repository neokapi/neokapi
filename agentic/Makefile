.PHONY: up down logs status \
	up-excalidraw logs-excalidraw \
	shell-excalidraw-alex shell-excalidraw-sophie \
	shell-excalidraw-thomas shell-excalidraw-mei

# === Stack ===

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

status:
	docker compose ps

# === Excalidraw Workspace ===

EXCALIDRAW_AGENTS = excalidraw-alex excalidraw-sophie excalidraw-thomas excalidraw-mei

up-excalidraw:
	docker compose up -d $(EXCALIDRAW_AGENTS)

logs-excalidraw:
	docker compose logs -f $(EXCALIDRAW_AGENTS)

shell-excalidraw-alex:
	docker compose exec excalidraw-alex /bin/sh

shell-excalidraw-sophie:
	docker compose exec excalidraw-sophie /bin/sh

shell-excalidraw-thomas:
	docker compose exec excalidraw-thomas /bin/sh

shell-excalidraw-mei:
	docker compose exec excalidraw-mei /bin/sh
