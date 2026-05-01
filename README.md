# TrackFlow

![CI](https://github.com/int28t/trackflow/actions/workflows/ci.yml/badge.svg?branch=main)

![Release](https://img.shields.io/github/v/tag/int28t/trackflow?style=for-the-badge)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Coverage](https://img.shields.io/badge/test%20coverage-45%25-yellow?style=for-the-badge)
![Docker](https://img.shields.io/badge/Docker-2496ED?style=for-the-badge&logo=docker&logoColor=white)

📄 [Презентация с защиты проекта](docs/presentation.pdf)

TrackFlow - учебный микросервисный проект по логистическому трекингу заказов.

Идея простая: через единый публичный API можно создать заказ, назначить курьера, вести статусы доставки и смотреть таймлайн событий.

## Состав сервисов

| Сервис | Роль | Внутренний порт |
|---|---|---|
| nginx | Единая точка входа в систему | 80 |
| api-gateway | Публичные endpoint, валидация и маршрутизация | 8081 |
| order-service | Создание/чтение заказа, назначение курьера | 8082 |
| tracking-service | История и обновления статусов | 8083 |
| carrier-sync-service | Пуллинг статусов перевозчика и отправка в tracking | 8084 |
| notification-service | Отправка уведомлений (в проекте используется mock sender) | 8085 |
| postgres | Основная БД | 5432 |
| redis | Кэш | 6379 |

## Быстрый запуск

Требования:

- Docker + Docker Compose
- Go 1.25.x (если запускать команды локально, не в контейнере)

Шаги:

1. Подготовить окружение:

```bash
cp .env.example .env
```

2. Поднять стек:

```bash
docker compose up -d --build
```

3. Применить схему БД:

```bash
docker compose exec -T postgres psql -U trackflow -d trackflow < migrations/postgres/000001_schema_v1.up.sql
```

4. (Опционально) загрузить демо-данные:

```bash
docker compose exec -T postgres psql -U trackflow -d trackflow < migrations/postgres/000002_seed_demo_v1.up.sql
```

5. Запустить smoke локального стека:

```bash
./scripts/smoke-local-stack.sh --skip-build
```

6. Проверить health:

```bash
curl -fsS http://127.0.0.1:8080/health
```

7. Остановить стек:

```bash
docker compose down
```

## Команды разработки

Через Makefile:

```bash
make help
make run SERVICE=api-gateway
make run SERVICE=order-service
make test
make lint
```

Прогон тестов по всем сервисам:

```bash
for svc in api-gateway order-service tracking-service carrier-sync-service notification-service; do
  go test ./services/$svc/...
done
```

Прогон покрытия:

```bash
for svc in api-gateway order-service tracking-service carrier-sync-service notification-service; do
  go test ./services/$svc/... -cover
done
```

## Smoke и e2e

Smoke для локального стека:

```bash
./scripts/smoke-local-stack.sh
```

Полезные флаги:

- --skip-build
- --down

E2E happy path через gateway:

```bash
./scripts/smoke-gateway-happy-path.sh
```

Опциональные переменные:

- E2E_GATEWAY_BASE_URL (по умолчанию http://127.0.0.1:8080)
- E2E_COURIER_ID (по умолчанию seeded courier id)

## API и Swagger

OpenAPI контракт:

- docs/api/openapi-v1.yaml

Swagger UI через Docker:

```bash
./scripts/swagger-ui-local.sh
```

URL после старта:

- http://127.0.0.1:8089

Параметры запуска:

- SWAGGER_UI_PORT (default 8089)
- OPENAPI_FILE (default docs/api/openapi-v1.yaml)

Если Docker недоступен, можно поднять статику локально:

```bash
python3 -m http.server 8090
```

и открыть:

- http://127.0.0.1:8090/docs/api/swagger-ui/

## CI/CD

Pipeline в GitLab включает:

- build;
- test;
- coverage + gate;
- compose-smoke.

Coverage gate:

- для всех микросервисов, кроме api-gateway, порог не ниже 30%;
- при падении ниже порога pipeline завершается ошибкой.

Артефакты покрытия сохраняются в папку coverage, включая Cobertura XML для UI GitLab.
