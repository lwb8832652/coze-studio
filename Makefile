.PHONY: debug fe server db_local_up db_local_migrate sync_db dump_db sql_init middleware web down clean python help

# 定义脚本路径
SCRIPTS_DIR := ./scripts
BUILD_FE_SCRIPT := $(SCRIPTS_DIR)/build_fe.sh
BUILD_SERVER_SCRIPT := $(SCRIPTS_DIR)/setup/server.sh
SETUP_DOCKER_SCRIPT := $(SCRIPTS_DIR)/setup/docker.sh
SETUP_PYTHON_SCRIPT := $(SCRIPTS_DIR)/setup/python.sh
COMPOSE_FILE := docker/docker-compose-debug.yml
OCEANBASE_COMPOSE_FILE := docker/docker-compose-oceanbase.yml
OCEANBASE_DEBUG_COMPOSE_FILE := docker/docker-compose-oceanbase_debug.yml
ENV_FILE := ./docker/.env.debug
WEB_ENV_FILE := ./docker/.env
OCEANBASE_ENV_FILE := ./docker/.env.debug
STATIC_DIR := ./bin/resources/static
ES_INDEX_SCHEMA := ./docker/volumes/elasticsearch/es_index_schema
ES_SETUP_SCRIPT := ./docker/volumes/elasticsearch/setup_es.sh

debug: env middleware python server

env:
	@if [ ! -f "$(ENV_FILE)" ]; then \
		echo "Env file '$(ENV_FILE)' not found, using example env..."; \
		cp ./docker/.env.debug.example $(ENV_FILE); \
	fi
	@bash ./scripts/setup/ensure_object_storage_credential_key.sh "$(ENV_FILE)"
	@tmp_file="$$(mktemp)"; \
	grep -Ev '^(# Agent thread runtime for P0 debug validation|(export[[:space:]]+)?(AGENT_THREAD_RUNTIME_DEFAULT|AGENT_THREAD_EINO_ADK_ENABLED|AGENT_THREAD_WORKER_ENABLED|AGENT_THREAD_WORKER_ID|AGENT_THREAD_WORKER_BATCH_SIZE|AGENT_THREAD_WORKER_INTERVAL_MS)=)' "$(ENV_FILE)" > "$$tmp_file"; \
	cat "$$tmp_file" > "$(ENV_FILE)"; \
	rm -f "$$tmp_file"; \
	{ \
		echo ""; \
		echo "# Agent thread runtime for P0 debug validation"; \
		echo "export AGENT_THREAD_RUNTIME_DEFAULT=eino_adk"; \
		echo "export AGENT_THREAD_EINO_ADK_ENABLED=true"; \
		echo "export AGENT_THREAD_WORKER_ENABLED=true"; \
		echo "export AGENT_THREAD_WORKER_ID=agent-run-worker"; \
		echo "export AGENT_THREAD_WORKER_BATCH_SIZE=10"; \
		echo "export AGENT_THREAD_WORKER_INTERVAL_MS=2000"; \
	} >> "$(ENV_FILE)"

fe:
	@echo "Building frontend..."
	@bash $(BUILD_FE_SCRIPT)

server: env
	@if [ ! -d "$(STATIC_DIR)" ]; then \
		echo "Static directory '$(STATIC_DIR)' not found, building frontend..."; \
		$(MAKE) fe; \
	fi
	@echo "Building and run server..."
	@APP_ENV=debug bash $(BUILD_SERVER_SCRIPT) -start


build_server:
	@echo "Building server..."
	@bash $(BUILD_SERVER_SCRIPT)

db_local_up: env
	@echo "Start isolated local MySQL without applying schema changes"
	@docker compose -f $(COMPOSE_FILE) --env-file $(ENV_FILE) --profile local-mysql up -d mysql --wait

db_local_migrate: db_local_up
	@echo "Apply versioned migrations only to the Compose-local mysql:3306 database"
	@docker compose -f $(COMPOSE_FILE) --env-file $(ENV_FILE) --profile local-db-migrate run --rm mysql-migrate-local

sync_db:
	@echo "sync_db 已停用；本地数据库请运行 make db_local_migrate，远程 dev 请运行 deploy/dev/publish-dev.sh。"
	@exit 1

dump_db:
	@echo "dump_db 已停用；docker/atlas/migrations 是结构事实源，请新增版本化 migration。"
	@exit 1

sql_init:
	@echo "sql_init 已停用；共享 dev 数据更新必须提交 data migration 并走发布流程。"
	@exit 1

middleware:
	@echo "Start middleware docker environment for opencoze app"
	@docker compose -f $(COMPOSE_FILE) --env-file $(ENV_FILE) --profile middleware up -d --wait

build_docker:
	@echo "Build docker image"
	@docker compose -f $(COMPOSE_FILE) --profile build-server build

web_env:
	@if [ ! -f "$(WEB_ENV_FILE)" ]; then \
		echo "Env file '$(WEB_ENV_FILE)' not found, using example env..."; \
		cp ./docker/.env.example $(WEB_ENV_FILE); \
	fi
	@bash ./scripts/setup/ensure_object_storage_credential_key.sh "$(WEB_ENV_FILE)"

web: web_env
	@echo "Start web server in docker"
	@docker compose -f docker/docker-compose.yml --env-file $(WEB_ENV_FILE) up -d

down_web:
	@echo "Stop web server in docker"
	@docker compose -f docker/docker-compose.yml --env-file $(WEB_ENV_FILE) down

down: env
	@echo "Stop all docker containers"
	@docker compose -f $(COMPOSE_FILE) --profile '*' down

clean: down
	@echo "Remove docker containers and volumes data"
	@rm -rf ./docker/data

python:
	@echo "Setting up Python..."
	@bash $(SETUP_PYTHON_SCRIPT)

dump_sql_schema:
	@echo "dump_sql_schema 已停用；禁止从共享数据库反向生成可执行 schema 快照。"
	@exit 1

atlas-hash:
	@echo "Rehash atlas migration files..."
	@(cd ./docker/atlas && atlas migrate hash)

setup_es_index:
	@echo "Setting up Elasticsearch index..."
	@. $(ENV_FILE); \
	bash $(ES_SETUP_SCRIPT) --index-dir $(ES_INDEX_SCHEMA) --docker-host false --es-address "$$ES_ADDR"

oceanbase_env:
	@bash scripts/setup/oceanbase_env.sh debug

oceanbase_debug: oceanbase_env oceanbase_middleware_debug python oceanbase_server_debug

oceanbase_middleware_debug:
	@echo "Starting OceanBase debug middleware..."
	@docker compose -f $(OCEANBASE_DEBUG_COMPOSE_FILE) --env-file $(ENV_FILE) --profile middleware up -d --wait

oceanbase_server_debug:
	@if [ ! -d "$(STATIC_DIR)" ]; then \
		echo "Static directory '$(STATIC_DIR)' not found, building frontend..."; \
		$(MAKE) fe; \
	fi
	@echo "Building and run OceanBase debug server..."
	@APP_ENV=debug bash $(BUILD_SERVER_SCRIPT) -start

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  debug            - Start the debug environment."
	@echo "  env              - Setup env file."
	@echo "  fe               - Build the frontend."
	@echo "  server           - Build and run the server binary."
	@echo "  build_server     - Build the server binary."
	@echo "  db_local_up      - Start the isolated Compose-local MySQL without DDL."
	@echo "  db_local_migrate - Explicitly apply versioned migrations to local mysql:3306."
	@echo "  sync_db          - Disabled unsafe legacy schema synchronization target."
	@echo "  dump_db          - Disabled legacy schema snapshot target."
	@echo "  sql_init         - Disabled direct SQL initialization target."
	@echo "  dump_sql_schema  - Disabled reverse schema snapshot target."
	@echo "  middleware       - Setup middlewares docker environment, but exclude the server app."
	@echo "  web              - Setup web docker environment, include middlewares docker."
	@echo "  down             - Stop the docker containers."
	@echo "  down_web         - Stop the web docker containers."
	@echo "  clean            - Stop the docker containers and clean volumes."
	@echo "  python           - Setup python environment."
	@echo "  atlas-hash       - Rehash atlas migration files."
	@echo "  setup_es_index   - Setup elasticsearch index."
	@echo ""
	@echo "OceanBase Commands:"
	@echo "  oceanbase_env    - Setup OceanBase environment file (like 'env')."
	@echo "  oceanbase_debug  - Start OceanBase debug environment (like 'debug')."
	@echo ""
	@echo "  help             - Show this help message."
