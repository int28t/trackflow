# TrackFlow

TrackFlow - микросервисный проект для логистического трекинга.

Внешний трафик проходит через Nginx, после чего API Gateway маршрутизирует запросы во внутренние сервисы.

## Структура сервисов

| Сервис | Назначение | Внутренний порт |
|---|---|---|
| nginx | Единая внешняя точка входа, reverse proxy на gateway | 80 |
| api-gateway | Публичный REST API, валидация, маршрутизация, единый формат ошибок | 8081 |
| order-service | Создание заказов, список заказов, назначение курьера, чтение заказа | 8082 |
| tracking-service | Обновление статусов и чтение таймлайна заказа | 8083 |
| carrier-sync-service | Синхронизация статусов от перевозчика и отправка в tracking-service | 8084 |
| notification-service | Отправка уведомлений (по умолчанию mock provider) | 8085 |
| postgres | Основное SQL-хранилище | 5432 |
| redis | Кэш | 6379 |

Снаружи Docker проброшен только Nginx (host-порт по умолчанию: 8080).

## Требования

- Docker + Docker Compose
- Go 1.25.x (для локальных команд вне контейнеров)

## Быстрый запуск (Docker Compose)

1. Создать файл окружения:

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

4. Опционально загрузить demo seed (курьеры, заказы, история статусов):

```bash
docker compose exec -T postgres psql -U trackflow -d trackflow < migrations/postgres/000002_seed_demo_v1.up.sql
```

5. Выполнить smoke-проверку локального стека:

```bash
./scripts/smoke-local-stack.sh --skip-build
```

6. Проверить health endpoint:

```bash
curl -fsS http://127.0.0.1:8080/health
```

7. Остановить стек:

```bash
docker compose down
```

## Основные команды

### Короткие команды через Makefile

```bash
make help
make run SERVICE=api-gateway
make run SERVICE=order-service
make test
make lint
```

### Цикл тестов по сервисам

```bash
for svc in api-gateway order-service tracking-service carrier-sync-service notification-service; do
  go test ./services/$svc/...
done
```

### Цикл покрытия по сервисам

```bash
for svc in api-gateway order-service tracking-service carrier-sync-service notification-service; do
  go test ./services/$svc/... -cover
done
```

## Smoke и E2E

### Health smoke для локального стека

```bash
./scripts/smoke-local-stack.sh
```

Полезные флаги:

- `--skip-build` запуск без пересборки образов
- `--down` остановка стека после завершения smoke

### E2E smoke happy-path через gateway

```bash
./scripts/smoke-gateway-happy-path.sh
```

Опциональные переменные окружения:

- `E2E_GATEWAY_BASE_URL` (по умолчанию `http://127.0.0.1:8080`)
- `E2E_COURIER_ID` (по умолчанию seeded courier id)

## API контракт

- Базовый публичный контракт: `docs/api/openapi-v1.yaml`

### Swagger UI

Быстрый запуск через Docker:

```bash
./scripts/swagger-ui-local.sh
```

После запуска открыть:

```text
http://127.0.0.1:8089
```

Опциональные переменные:

- `SWAGGER_UI_PORT` (по умолчанию `8089`)
- `OPENAPI_FILE` (по умолчанию `docs/api/openapi-v1.yaml`)

Вариант без Docker (через локальный HTTP-сервер):

```bash
python3 -m http.server 8090
```

И затем открыть страницу:

```text
http://127.0.0.1:8090/docs/api/swagger-ui/
```

Основные gateway маршруты:

- `GET /health`
- `GET /metrics`
- `GET/POST /v1/orders`
- `GET /v1/orders/{id}`
- `POST /v1/orders/{id}/assign`
- `POST /v1/orders/{id}/status`
- `GET /v1/orders/{id}/timeline`

## Структура репозитория

```text
trackflow/
  services/
    api-gateway/
    order-service/
    tracking-service/
    carrier-sync-service/
    notification-service/
  migrations/
    postgres/
  deploy/
    nginx/
  scripts/
    smoke-local-stack.sh
    smoke-gateway-happy-path.sh
    swagger-ui-local.sh
  docs/
    api/openapi-v1.yaml
    api/swagger-ui/index.html
  docker-compose.yml
  .gitlab-ci.yml
  Makefile
```

## CI/CD

GitLab CI pipeline включает:

- Стадию build для всех сервисов
- Стадию test для всех сервисов
- Стадию coverage с отчетами и coverage gate
- Стадию compose-smoke с проверкой health-check

Политика coverage gate:

- Для каждого микросервиса, кроме `api-gateway`, покрытие должно быть >= 30%
- Если у любого non-gateway сервиса покрытие ниже порога, pipeline завершается ошибкой

Артефакты покрытия собираются в `coverage/`, включая Cobertura XML для GitLab UI.
