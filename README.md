# tco

Локальный Go-сервис, который забирает историю сообщений из Telegram-чата, считает по ним локальные ONNX-эмбеддинги, дедуплицирует и кластеризует записи и раскладывает результат в Obsidian-совместимый vault (Markdown + JSON-манифест).

Пайплайн: `Telegram → ONNX embeddings → dedup/clustering → Markdown + manifest`.

Всё работает без внешних LLM-сервисов и без отправки сообщений наружу: модель исполняется локально через `onnxruntime`.

## Оглавление

- [Что делает сервис](#что-делает-сервис)
- [Минимальные требования](#минимальные-требования)
- [Быстрый старт](#быстрый-старт)
- [Получение Telegram API credentials](#получение-telegram-api-credentials)
- [Подготовка модели и onnxruntime](#подготовка-модели-и-onnxruntime)
- [Файл `.env`](#файл-env)
- [Сборка и запуск](#сборка-и-запуск)
- [HTTP control plane](#http-control-plane)
- [Полный сценарий использования](#полный-сценарий-использования)
- [Структура vault](#структура-vault)
- [Диагностика и типичные ошибки](#диагностика-и-типичные-ошибки)
- [Структура репозитория](#структура-репозитория)

## Что делает сервис

- Подключается к Telegram как обычный пользовательский клиент через MTProto (библиотека [`gotd/td`](https://github.com/gotd/td)).
- Забирает историю указанного чата/канала/личного диалога.
- Каждое сообщение токенизируется и прогоняется через ONNX-модель (по умолчанию BERT-подобный энкодер `all-MiniLM-L6-v2` с mean-pooling и L2-нормализацией).
- Похожие сообщения (порог `DEDUP_SIMILARITY_THRESHOLD`) помечаются как дубликаты, остальные объединяются в кластеры (`CLUSTER_SIMILARITY_THRESHOLD`).
- Итог сохраняется в `vault/` как набор Markdown-заметок и `manifest.json`; эмбеддинги — рядом, в `vault/_meta/embeddings/`.
- Управление — через локальный HTTP control plane: `/auth` (web-onboarding), `/pipeline/run`, `/pipeline/status`, `/healthz`, `/readyz`.

## Минимальные требования

- **ОС:** Linux x86_64 (`amd64`). Скрипт `scripts/install_onnx.sh` проверяет `uname -s == Linux` и `uname -m in {x86_64, amd64}` и на других системах завершится с ошибкой. На macOS/Windows/ARM можно запуститься, но `libonnxruntime` придётся ставить вручную — не тестировалось.
- **Go:** `1.26.2`.
- **Toolchain:** `git`, `curl`, `tar`, `bash`.
- **Сеть:** прямой доступ к Telegram DC или рабочий SOCKS5-прокси (`TELEGRAM_PROXY_ADDR`).
- **Telegram-аккаунт + API credentials** (`TELEGRAM_API_ID`, `TELEGRAM_API_HASH`) — выдаются на [my.telegram.org](https://my.telegram.org).
- **Модельные артефакты** в каталоге `./models` относительно запуска бинаря:
  - `all-MiniLM-L6-v2.onnx` — веса модели;
  - `tokenizer.json` — токенизатор в формате HuggingFace `tokenizers` (WordPiece/BertWordPiece);
  - `libonnxruntime.so` — shared library `onnxruntime` для Linux x64.
- **Диск:** порядка 100–200 МБ на модель + runtime + результат.

## Быстрый старт

```bash
git clone https://github.com/flexer2006/tco.git
cd tco

# 1. Поставить onnxruntime (libonnxruntime.so → ./models/)
bash scripts/install_onnx.sh

# 2. Положить модель и токенизатор в ./models/
#    (см. раздел «Подготовка модели и onnxruntime»)
#    ./models/all-MiniLM-L6-v2.onnx
#    ./models/tokenizer.json

# 3. Подготовить окружение
cp .env.example .env
# отредактировать .env: TELEGRAM_API_ID, TELEGRAM_API_HASH, TELEGRAM_CHAT_ID

# 4. Собрать бинарь
go build -o bin/collector ./cmd/collector

# 5. Запустить
set -a; source .env; set +a
./bin/collector
```

Дальше — открыть `http://127.0.0.1:8080/auth`, пройти логин, и дернуть `POST /pipeline/run`.

## Получение Telegram API credentials

1. Открыть [https://my.telegram.org](https://my.telegram.org) и войти по номеру телефона.
2. Раздел **API development tools** → **Create new application**.
3. Заполнить поля (App title, Short name, Platform — `Desktop` годится). Поле URL не обязательно.
4. После создания страница покажет:
   - `App api_id` — это `TELEGRAM_API_ID` (число).
   - `App api_hash` — это `TELEGRAM_API_HASH` (hex-строка).
5. Хранить эти значения как секреты: одна пара на аккаунт, отзыв — там же.

Для `TELEGRAM_CHAT_ID` допустимые форматы (парсер — `internal/adapters/telegram_live_target.go`):

| Формат                                  | Что означает                                      |
|-----------------------------------------|---------------------------------------------------|
| `@username`                             | публичный юзернейм (канал, бот, чат, пользователь) |
| `username:<name>`                       | то же, но явно                                    |
| `123456789`                             | id классического чата (Basic Group), положительное |
| `-1001234567890`                        | peer-id супергруппы/канала (префикс `-100`)        |
| `chat:<id>`                             | явное указание chat id                            |
| `user:<id>:<access_hash>`               | личный диалог с доступом через access_hash        |
| `channel:<id>:<access_hash>`            | канал/супергруппа через access_hash               |

Простой способ узнать peer-id — переслать любое сообщение из нужного чата в бот [@getidsbot](https://t.me/getidsbot) или [@userinfobot](https://t.me/userinfobot).

## Подготовка модели и onnxruntime

### 1. `libonnxruntime.so`

В репозитории есть `scripts/install_onnx.sh`. Он:

- проверяет, что система Linux x86_64;
- скачивает релиз `onnxruntime-linux-x64-${ORT_VERSION}.tgz` (по умолчанию `1.24.1`) с GitHub Microsoft;
- извлекает `libonnxruntime.so` и кладёт в `./models/libonnxruntime.so`.

```bash
bash scripts/install_onnx.sh
# либо с конкретной версией:
ORT_VERSION=1.20.1 bash scripts/install_onnx.sh
```

> Протестировано только на Linux amd64. На других платформах (macOS/ARM/Windows) скрипт работать не будет — нужно вручную скачать соответствующий релиз [microsoft/onnxruntime](https://github.com/microsoft/onnxruntime/releases) и положить `libonnxruntime.so`/`.dylib`/`.dll` в `./models/`, затем прописать путь в `ONNXRUNTIME_SHARED_LIBRARY`.

### 2. Модель и токенизатор

По умолчанию ожидается `all-MiniLM-L6-v2` в ONNX-формате + соответствующий `tokenizer.json` (WordPiece).

Самый быстрый способ — взять уже сконвертированный вариант с HuggingFace, например:

```bash
mkdir -p models
# пример: репозиторий Xenova/all-MiniLM-L6-v2 содержит onnx + tokenizer.json
curl -fL -o models/all-MiniLM-L6-v2.onnx \
  https://huggingface.co/Xenova/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx
curl -fL -o models/tokenizer.json \
  https://huggingface.co/Xenova/all-MiniLM-L6-v2/resolve/main/tokenizer.json
```

Альтернативно — сконвертировать самому из оригинала `sentence-transformers/all-MiniLM-L6-v2`:

```bash
pip install "optimum[onnxruntime]" sentence-transformers
optimum-cli export onnx \
  --model sentence-transformers/all-MiniLM-L6-v2 \
  --task feature-extraction \
  models_src
cp models_src/model.onnx    models/all-MiniLM-L6-v2.onnx
cp models_src/tokenizer.json models/tokenizer.json
```

Подойдёт и другая BERT-совместимая модель (например, `multi-qa-MiniLM-L6-cos-v1`, `paraphrase-multilingual-MiniLM-L12-v2`, `bge-small-en`). Требования:

- модель экспортируется в ONNX как feature-extraction (входы `input_ids`, `attention_mask`, `token_type_ids`, выход — последний hidden state);
- наличие `tokenizer.json` с моделью `WordPiece`/`BertWordPiece` и токенами `[CLS]`, `[SEP]`, `[UNK]`;
- известна размерность эмбеддинга — её надо выставить в `EMBED_VECTOR_DIMENSION` (384 у MiniLM-L6-v2, 768 у base-моделей).

Для моделей, у которых ONNX уже выдаёт готовый вектор на строку без токенизатора (редкий случай), есть профиль `EMBED_MODEL_PROFILE=string_input_direct` — тогда `tokenizer.json` не нужен, но нужен ONNX с входом типа `string` и выходом фиксированной размерности.

## Файл `.env`

Шаблон — `.env.example`. Загружать можно как угодно: `set -a; source .env; set +a`, `direnv`, `env $(cat .env | xargs)` и т. п. Сервис читает только переменные окружения процесса, сам `.env` он не парсит.

### Обязательные

| Переменная             | Назначение                                                                   |
|------------------------|------------------------------------------------------------------------------|
| `TELEGRAM_API_ID`      | `api_id` с my.telegram.org (число).                                          |
| `TELEGRAM_API_HASH`    | `api_hash` с my.telegram.org.                                                |
| `TELEGRAM_CHAT_ID`     | Идентификатор источника (см. таблицу выше).                                  |

Отсутствие любой из этих трёх переменных — `collector` падает на старте с понятной ошибкой.

### HTTP control plane

| Переменная    | Значение по умолчанию | Что делает                                                                 |
|---------------|-----------------------|---------------------------------------------------------------------------|
| `HTTP_BIND`   | `127.0.0.1`           | Адрес, на котором слушает control plane. **Не биндить на `0.0.0.0` без причин** — эндпоинты `/auth/*` принимают реальные Telegram-креды. |
| `HTTP_PORT`   | `8080`                | TCP-порт control plane (1..65535).                                        |

### Vault и манифест

| Переменная        | Значение по умолчанию                | Что делает                                                         |
|-------------------|--------------------------------------|--------------------------------------------------------------------|
| `VAULT_ROOT`      | `./vault`                            | Корневой каталог результата (Markdown + meta).                     |
| `MANIFEST_PATH`   | `${VAULT_ROOT}/_meta/manifest.json`  | Путь к `manifest.json`. Автогенерируется при первом прогоне.       |

### Telegram runtime

| Переменная               | Значение по умолчанию                     | Что делает                                                                                                           |
|--------------------------|-------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `RUNTIME_PROFILE`        | `real`                                    | Профиль рантайма. Поддерживается только `real`.                                                                      |
| `TELEGRAM_SOURCE_MODE`   | `live`                                    | Источник сообщений. Поддерживается только `live` (боевой MTProto-клиент).                                            |
| `TELEGRAM_SESSION_PATH`  | `${VAULT_ROOT}/_meta/telegram.session.json` | Файл с сессией Telegram. Создаётся после успешной авторизации, дальше переиспользуется.                              |
| `TELEGRAM_PROXY_ADDR`    | *пусто*                                   | Необязательный SOCKS5-прокси в формате `host:port`.                                                                  |

### Пайплайн и эмбеддинги

| Переменная                       | Значение по умолчанию                        | Что делает                                                                                                         |
|----------------------------------|----------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| `RUN_MODE`                       | `incremental`                                | Режим прогона: `incremental` (сравнение с существующим манифестом) или `full_rebuild` (пересбор с нуля).           |
| `BATCH_MODE`                     | `streaming`                                  | `streaming` — кодировать по мере получения; `post_scan` — сначала собрать все сообщения, затем кодировать.         |
| `BATCH_SIZE`                     | `32`                                         | Размер батча для энкодера (>0).                                                                                    |
| `EMBED_MODEL_ID`                 | `all-MiniLM-L6-v2-go`                        | Логический идентификатор модели, попадает в `manifest.json`.                                                       |
| `EMBED_MODEL_PATH`               | `./models/all-MiniLM-L6-v2.onnx`             | Путь до `.onnx`-файла.                                                                                             |
| `EMBED_MODEL_PROFILE`            | `bert_tokenized_mean_pooling`                | Профиль инференса: `bert_tokenized_mean_pooling` (BERT + mean pool + L2) или `string_input_direct` (ONNX сам всё делает). |
| `EMBED_VECTOR_DIMENSION`         | `384`                                        | Размерность выходного вектора. Должна совпадать с моделью (MiniLM-L6-v2 → 384).                                    |
| `ONNXRUNTIME_SHARED_LIBRARY`     | `./models/libonnxruntime.so`                 | Путь к shared library `onnxruntime`.                                                                               |
| `ONNX_INPUT_NAME`                | *auto*                                       | Переопределение имени входного тензора модели. Пусто — определяется автоматически.                                 |
| `ONNX_OUTPUT_NAME`               | *auto*                                       | Переопределение имени выходного тензора модели. Пусто — автоматически.                                             |
| `DEDUP_SIMILARITY_THRESHOLD`     | `0.95`                                       | Порог косинусной близости для дедупликации, диапазон `(0; 1]`.                                                     |
| `CLUSTER_SIMILARITY_THRESHOLD`   | `0.80`                                       | Порог для кластеризации, диапазон `(0; 1]`. Обычно меньше дедуп-порога.                                            |

### Прочее

Переменные `LOG_LEVEL`, `TELEGRAM_AUTH_TIMEOUT`, `TELEGRAM_FIXTURE_PATH`, `TELEGRAM_INCLUDE_ALL_MESSAGES` лежат в `.env.example` для совместимости, но в текущей сборке `config.Load` их не читает — менять их смысла нет.

## Сборка и запуск

### Сборка

```bash
go mod download
go build -o bin/collector ./cmd/collector
```

Проверки, которые имеет смысл гонять:

```bash
go vet ./...
go build ./...
```

### Запуск

`libonnxruntime.so` подгружается по абсолютному или относительному (относительно CWD процесса) пути из `ONNXRUNTIME_SHARED_LIBRARY`. Проще всего запускать из корня репозитория, где лежит каталог `./models`.

```bash
set -a
source .env
set +a

./bin/collector
```

Остановка — `Ctrl+C` (SIGINT) или `kill <pid>` (SIGTERM). Сервис корректно завершает HTTP-сервер и сохраняет состояние.

## HTTP control plane

Все эндпоинты отвечают `application/json` (кроме `/auth`, который отдаёт HTML). CSRF защищён cookie + скрытым полем формы; на `POST /auth/*` нужно либо ходить со страницы `/auth`, либо передавать оба значения вручную.

| Метод  | Путь                    | Назначение                                                                     |
|--------|-------------------------|--------------------------------------------------------------------------------|
| `GET`  | `/healthz`              | Живость процесса. `200` всегда, если процесс жив.                              |
| `GET`  | `/readyz`               | Готовность: и пайплайн, и onboarding должны быть готовы, иначе `503` + причина.|
| `GET`  | `/auth`                 | HTML-страница onboarding'а (логин/код/2FA).                                    |
| `GET`  | `/auth/state`           | Текущее состояние авторизации в JSON (snapshot + readiness).                   |
| `POST` | `/auth/start`           | Форма: `api_id`, `api_hash`, `phone` (E.164). Инициирует логин.                |
| `POST` | `/auth/verify-code`     | Форма: `code` — код из Telegram.                                               |
| `POST` | `/auth/verify-password` | Форма: `password` — пароль 2FA (cloud password), если включён.                 |
| `POST` | `/auth/logout`          | Сброс Telegram-сессии.                                                         |
| `POST` | `/pipeline/run`         | Запуск полного прогона пайплайна. `202` — принято, `409` — уже запущен.        |
| `GET`  | `/pipeline/status`      | Статус последнего/текущего прогона в JSON.                                     |

## Полный сценарий использования

Ниже — шаги «с нуля до готового vault». Предполагается, что `.env` заполнен, бинарь собран, модель и runtime лежат в `./models/`.

### 1. Запустить сервис

В первом терминале:

```bash
cd /path/to/tco
set -a; source .env; set +a
./bin/collector
```

В логах должен появиться HTTP-листнер на `127.0.0.1:8080`.

### 2. Проверить живость и готовность

Во втором терминале:

```bash
curl -fsS http://127.0.0.1:8080/healthz
# {"status":"ok"}

curl -fsS http://127.0.0.1:8080/readyz
# до авторизации: 503 + {"status":"not_ready","reason":"..."}
```

### 3. Авторизоваться в Telegram

Самый простой путь — браузер: открыть `http://127.0.0.1:8080/auth` и заполнить форму.

Пошагово:

1. **Start login.** Ввести `API ID`, `API Hash`, `Phone` (в E.164, например `+79991234567`). Нажать **Start**. Telegram отправит код в ваш Telegram (в чат `Telegram` или через SMS).
2. **Verify code.** Ввести полученный код.
3. **Verify password.** Только если на аккаунте включён 2FA (cloud password) — ввести пароль. Иначе шаг пропускается.

После успешной авторизации:

- появится файл `TELEGRAM_SESSION_PATH` (по умолчанию `vault/_meta/telegram.session.json`);
- `GET /auth/state` вернёт `readiness.ready = true`;
- `GET /readyz` станет `200 ok`.

Быстрая проверка состояния без браузера:

```bash
curl -fsS http://127.0.0.1:8080/auth/state | jq
```

Сессия сохраняется; повторный запуск сервиса не потребует повторного логина, пока Telegram её не инвалидирует.

### 4. Запустить пайплайн

```bash
curl -X POST http://127.0.0.1:8080/pipeline/run
# {"status":"accepted","pipeline":{...}}
```

Повторный вызов во время активного прогона вернёт `409 Conflict`.

### 5. Наблюдать за прогоном в реальном времени

В терминале с сервисом идут структурированные логи этапов (fetch history → encode → dedup → cluster → manifest → projection). Параллельно можно опрашивать статус:

```bash
watch -n 1 'curl -fsS http://127.0.0.1:8080/pipeline/status | jq'
```

### 6. Посмотреть результат

После завершения прогона:

```bash
tree vault -L 3
cat vault/_meta/manifest.json | jq '.run_metadata'
ls vault/topics
```

Markdown-заметки можно открыть любым редактором или подхватить каталог `vault/` как vault в [Obsidian](https://obsidian.md/).

### 7. Повторные прогоны

- `RUN_MODE=incremental` (по умолчанию) — подхватывает существующий `manifest.json`, обрабатывает только изменения.
- `RUN_MODE=full_rebuild` — принудительный пересбор с нуля. Полезно, если сменили модель/пороги.

После изменения `.env` перезапустите бинарь и снова дёрните `POST /pipeline/run`.

## Структура vault

```
vault/
├── _meta/
│   ├── manifest.json               # основной манифест (записи, кластеры, metadata прогона)
│   ├── embeddings/
│   │   └── <embedding_id>.json     # по одному файлу на сообщение
│   └── telegram.session.json       # сохранённая Telegram-сессия
└── topics/
    └── <cluster>/
        ├── index.md                # сводка по кластеру
        └── <note>.md               # отдельные заметки
```

`manifest.json` содержит: параметры прогона, метаданные модели (id/хэш/размерность/правило нормализации), список `NoteRecord` и кластеры. Его можно скармливать внешним инструментам как single source of truth.

## Диагностика и типичные ошибки

| Симптом                                                                 | Причина / что делать                                                                                            |
|-------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| `TELEGRAM_API_ID: required environment variable is not set`             | Не экспортировали `.env`. Проверить: `env | grep TELEGRAM_`.                                                    |
| `EMBED_MODEL_PROFILE: ... unsupported value`                            | Допустимо только `bert_tokenized_mean_pooling` или `string_input_direct`.                                       |
| `initialize production ONNX encoder: model validation failed`           | Модель не найдена либо не читается. Проверить `EMBED_MODEL_PATH`.                                               |
| `onnxruntime: ... cannot open shared object file`                       | Нет `libonnxruntime.so`. Запустить `scripts/install_onnx.sh` или прописать путь в `ONNXRUNTIME_SHARED_LIBRARY`. |
| `tokenizer.json: unk_token "[UNK]" not found in vocab`                  | Не тот tokenizer.json (другая архитектура). Взять WordPiece/BertWordPiece-совместимый.                          |
| `runtime returned vector[i] with dimension X (expected Y)`              | `EMBED_VECTOR_DIMENSION` не совпадает с моделью.                                                                |
| `/readyz` отвечает `503 not_ready`                                      | Не завершён onboarding. Зайти на `/auth`, пройти логин/код/2FA.                                                 |
| `POST /pipeline/run → 409 conflict`                                     | Уже идёт прогон. Дождаться `/pipeline/status → idle/completed`.                                                 |
| `parse telegram live source chat ...: chat id must be positive`         | Для супергрупп/каналов нужен peer-id с префиксом `-100...`, либо `@username`.                                   |
| Логин зависает на «waiting for code»                                    | Код пришёл в SMS, а не в Telegram-клиент. Ввести его в форме `/auth`.                                           |

## Структура репозитория

```
.
├── cmd/collector/            # main — единственная точка входа
├── internal/
│   ├── adapters/             # ONNX runtime, токенизатор, Telegram MTProto-адаптер, HTTP control plane, vault-projector
│   ├── application/          # сценарии: pipeline orchestrator, onboarding, ports (интерфейсы)
│   ├── bootstrap/            # DI, сборка зависимостей, запуск HTTP-сервера
│   ├── config/               # парсинг/валидация окружения
│   └── domain/               # доменные типы: Manifest, Message, Note, Vector, Policy
├── scripts/install_onnx.sh   # установка libonnxruntime.so для Linux x64
├── .env.example              # шаблон окружения
├── go.mod / go.sum
└── README.md
```
